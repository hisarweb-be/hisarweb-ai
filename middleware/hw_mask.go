package middleware

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Provider masking. The customer may never see which supplier answered: no
// supplier name, no supplier header, no supplier response id and no supplier
// wording. Masking is structural (allowlisted headers, rewritten JSON/SSE
// payloads); the word scan exists only as a last-resort safety net.
const hwMaxPendingPayload = 4 << 20

// hwProviderNetEnabled toggles the last-resort scan. Tests disable it to prove
// the structural masking stands on its own.
var hwProviderNetEnabled atomic.Bool

func init() {
	hwProviderNetEnabled.Store(true)
}

// HwSetProviderNetEnabled switches the last-resort provider-word scan on or off.
func HwSetProviderNetEnabled(enabled bool) {
	hwProviderNetEnabled.Store(enabled)
}

// hwAllowedResponseHeaders lists the response headers a customer may receive.
// Everything else is dropped, which removes supplier traces by construction.
var hwAllowedResponseHeaders = map[string]bool{
	"content-type":        true,
	"content-length":      true,
	"cache-control":       true,
	"date":                true,
	"expires":             true,
	"pragma":              true,
	"vary":                true,
	"etag":                true,
	"last-modified":       true,
	"accept-ranges":       true,
	"content-range":       true,
	"content-disposition": true,
	"retry-after":         true,
	"x-accel-buffering":   true,
}

// hwContentKeys hold model-authored text. Masking them would rewrite the
// answer itself, so they are never touched by the last-resort word scan.
var hwContentKeys = map[string]bool{
	"content":           true,
	"text":              true,
	"thinking":          true,
	"reasoning":         true,
	"reasoning_content": true,
	"arguments":         true,
	"input":             true,
	"output_text":       true,
}

type hwMaskWriter struct {
	gin.ResponseWriter
	ctx *gin.Context

	// base holds the header names our own middleware set before masking
	// started; they survive the allowlist because they carry no supplier
	// information.
	base map[string]bool
	live http.Header

	status      int
	headerSeen  bool
	committed   bool
	transform   bool
	stream      bool
	anthropic   bool
	hijacked    bool
	flushed     bool
	lengthKnown bool
	buffer      bytes.Buffer
	pending     []byte
	ownModel    string
	requestID   string
}

// HwMask installs the customer-facing response mask. It belongs on every route a
// customer can call; admin-only routes do not need it.
func HwMask() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Writer == nil || hwMaskSkipped(c) {
			c.Next()
			return
		}
		if _, already := c.Writer.(*hwMaskWriter); already {
			c.Next()
			return
		}
		original := c.Writer
		writer := &hwMaskWriter{
			ResponseWriter: original,
			ctx:            c,
			base:           hwHeaderNameSet(original.Header()),
			live:           original.Header().Clone(),
			status:         http.StatusOK,
			ownModel:       c.GetString("original_model"),
			requestID:      c.GetString(common.RequestIdKey),
			anthropic:      hwAnthropicPath(c),
		}
		c.Writer = writer
		defer func() {
			if recovered := recover(); recovered != nil {
				c.Writer = original
				panic(recovered)
			}
			writer.finish()
			c.Writer = original
			service.HwRecordProviderAudit(c)
		}()
		c.Next()
	}
}

func hwHeaderNameSet(header http.Header) map[string]bool {
	names := make(map[string]bool, len(header))
	for name := range header {
		names[strings.ToLower(name)] = true
	}
	return names
}

// hwMaskSkipped reports whether a response must not be buffered or rewritten.
// The decision never looks at request headers: a customer can send "Upgrade" or
// "Connection: upgrade" on any route, and honouring those would hand out the
// unfiltered supplier answer. Only a connection the server itself hijacked is
// passed through, and the masking writer learns that from its own Hijack call.
func hwMaskSkipped(c *gin.Context) bool {
	return c == nil || c.Request == nil
}

func hwAnthropicPath(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	return strings.Contains(c.Request.URL.Path, "/messages")
}

func (w *hwMaskWriter) Header() http.Header {
	return w.live
}

func (w *hwMaskWriter) Status() int {
	return w.status
}

func (w *hwMaskWriter) Written() bool {
	return w.committed
}

func (w *hwMaskWriter) WriteHeader(code int) {
	if code <= 0 || w.headerSeen {
		return
	}
	w.headerSeen = true
	w.status = code
	if !w.planTransform() {
		w.commit()
	}
}

func (w *hwMaskWriter) WriteHeaderNow() {
	if !w.headerSeen {
		w.WriteHeader(http.StatusOK)
	}
}

func (w *hwMaskWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if !w.headerSeen {
		w.WriteHeader(http.StatusOK)
	}
	if w.hijacked {
		return w.ResponseWriter.Write(data)
	}
	if !w.transform || w.stream {
		// A rewritten body gets its headers (and length) at finish; a stream and
		// a pass-through body can go out immediately.
		w.commit()
	}
	if !w.transform {
		return w.ResponseWriter.Write(data)
	}
	if w.stream {
		service.HwNoteStreamWrite(w.ctx)
		if !hwLooksLikeSSE(data) {
			// The relay engine reports a terminal failure with a JSON envelope
			// while the client is already reading SSE; hand the client a proper
			// error event instead of a stray JSON document.
			data = w.streamErrorEvent(data)
		}
		out := w.sanitizeStream(data)
		if len(out) == 0 {
			return len(data), nil
		}
		_, err := w.ResponseWriter.Write(out)
		w.ResponseWriter.Flush()
		return len(data), err
	}
	w.buffer.Write(data)
	return len(data), nil
}

func (w *hwMaskWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *hwMaskWriter) Flush() {
	if !w.headerSeen {
		w.WriteHeader(http.StatusOK)
	}
	if !w.committed {
		w.commit()
	}
	w.flushed = true
	w.ResponseWriter.Flush()
}

// Hijack hands the connection to the protocol upgrade handler (WebSocket) and
// wraps its writer so the handshake response block is filtered before it leaves
// the process. Everything after the handshake — the WebSocket frames — is
// protocol data the HTTP mask cannot read, so it passes through unchanged; the
// handshake is the only place a supplier header could reach the customer.
func (w *hwMaskWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, readWriter, err := w.ResponseWriter.Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.hijacked = true
	readWriter.Writer = bufio.NewWriter(&hwHandshakeWriter{
		target: readWriter.Writer,
		ctx:    w.ctx,
	})
	return conn, readWriter, nil
}

// hwHandshakeWriter filters the status line and headers of a hijacked
// connection, then passes every following byte through untouched.
type hwHandshakeWriter struct {
	target io.Writer
	ctx    *gin.Context
	head   []byte
	open   bool
}

func (h *hwHandshakeWriter) Write(data []byte) (int, error) {
	if h.open {
		return h.target.Write(data)
	}
	h.head = append(h.head, data...)
	end := bytes.Index(h.head, []byte("\r\n\r\n"))
	if end < 0 {
		return len(data), nil
	}
	block := h.head[:end+4]
	rest := h.head[end+4:]
	h.head = nil
	h.open = true
	if _, err := h.target.Write(hwFilterHandshake(block)); err != nil {
		return 0, err
	}
	if len(rest) > 0 {
		if _, err := h.target.Write(rest); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

// hwFilterHandshake keeps the status line and drops every header line that names
// a supplier. Protocol headers (Upgrade, Connection, Sec-WebSocket-Accept) carry
// no supplier trace, so the handshake stays intact.
func hwFilterHandshake(block []byte) []byte {
	lines := bytes.Split(bytes.TrimSuffix(block, []byte("\r\n\r\n")), []byte("\r\n"))
	var out bytes.Buffer
	for index, line := range lines {
		if index == 0 {
			out.Write(line)
			out.WriteString("\r\n")
			continue
		}
		if len(line) == 0 {
			continue
		}
		name, value, found := bytes.Cut(line, []byte(":"))
		if !found {
			out.Write(line)
			out.WriteString("\r\n")
			continue
		}
		if common.HwHasProviderTrace(string(name)) {
			continue
		}
		out.Write(name)
		out.WriteString(":")
		out.WriteString(common.HwReplaceProviderWords(string(value)))
		out.WriteString("\r\n")
	}
	out.WriteString("\r\n")
	return out.Bytes()
}

// planTransform decides from the committed content type whether the body needs
// rewriting. Binary payloads pass through untouched; only their headers are
// filtered.
func (w *hwMaskWriter) planTransform() bool {
	contentType := strings.ToLower(w.live.Get("Content-Type"))
	w.stream = strings.HasPrefix(contentType, "text/event-stream")
	w.transform = w.stream ||
		strings.Contains(contentType, "json") ||
		strings.HasPrefix(contentType, "text/") ||
		contentType == ""
	return w.transform
}

// commit filters the response headers and writes the status line once.
func (w *hwMaskWriter) commit() {
	if w.committed {
		return
	}
	w.committed = true
	w.planTransform()
	target := w.ResponseWriter.Header()
	for key := range target {
		delete(target, key)
	}
	for key, values := range w.live {
		if !hwResponseHeaderAllowed(key, w.base) {
			continue
		}
		clean := make([]string, 0, len(values))
		for _, value := range values {
			clean = append(clean, hwSafetyNet(w.ctx, []byte(value)))
		}
		target[key] = clean
	}
	if w.requestID != "" && target.Get(common.RequestIdKey) == "" {
		target.Set(common.RequestIdKey, w.requestID)
	}
	if w.transform && (w.stream || !w.lengthKnown) {
		// Length is decided by the rewritten body, never by the supplier.
		target.Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(w.status)
}

// finish flushes whatever the handler produced. Streams end with a final flush;
// buffered bodies are sanitised as a whole so the length stays correct.
func (w *hwMaskWriter) finish() {
	if w.hijacked {
		return
	}
	if w.stream {
		if !w.headerSeen {
			w.WriteHeader(http.StatusOK)
		}
		if len(w.pending) > 0 {
			_, _ = w.ResponseWriter.Write([]byte(hwSafetyNet(w.ctx, w.pending)))
			w.pending = nil
		}
		w.ResponseWriter.Flush()
		return
	}
	body := w.buffer.Bytes()
	if w.transform {
		body = w.sanitizeBody(body)
	}
	if !w.headerSeen {
		w.WriteHeader(http.StatusOK)
	}
	if !w.committed {
		if w.transform {
			// The rewritten body decides the length, never the upstream header.
			if len(body) > 0 && !w.flushed {
				w.live.Set("Content-Length", strconv.Itoa(len(body)))
				w.lengthKnown = true
			} else {
				w.live.Del("Content-Length")
			}
		}
		w.commit()
	}
	if len(body) == 0 {
		return
	}
	_, _ = w.ResponseWriter.Write(body)
}

// sanitizeBody rewrites a complete non-stream response body.
func (w *hwMaskWriter) sanitizeBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var object map[string]any
	if err := common.Unmarshal(body, &object); err == nil && object != nil {
		w.sanitizeObject(object, w.status)
		if encoded, err := common.Marshal(object); err == nil {
			return encoded
		}
		return body
	}
	return []byte(hwSafetyNet(w.ctx, body))
}

// sanitizeStream rewrites the complete JSON values inside an SSE byte stream and
// forwards everything else (event names, comments, separators) verbatim.
func (w *hwMaskWriter) sanitizeStream(data []byte) []byte {
	w.pending = append(w.pending, data...)
	var out bytes.Buffer
	for len(w.pending) > 0 {
		start := hwJSONValueStart(w.pending)
		if start < 0 {
			out.WriteString(hwSafetyNet(w.ctx, w.pending))
			w.pending = nil
			break
		}
		if start > 0 {
			out.WriteString(hwSafetyNet(w.ctx, w.pending[:start]))
			w.pending = w.pending[start:]
		}
		end, complete := hwJSONValueEnd(w.pending)
		if !complete {
			if len(w.pending) > hwMaxPendingPayload {
				out.Write(w.pending)
				w.pending = nil
			}
			break
		}
		out.Write(w.sanitizePayload(w.pending[:end]))
		w.pending = w.pending[end:]
	}
	if out.Len() == 0 {
		return nil
	}
	return out.Bytes()
}

// sanitizePayload rewrites one complete JSON value from a stream.
func (w *hwMaskWriter) sanitizePayload(payload []byte) []byte {
	var object map[string]any
	if err := common.Unmarshal(payload, &object); err != nil || object == nil {
		return []byte(hwSafetyNet(w.ctx, payload))
	}
	status := w.status
	if status < http.StatusBadRequest {
		status = http.StatusBadGateway
	}
	w.sanitizeObject(object, status)
	encoded, err := common.Marshal(object)
	if err != nil {
		return payload
	}
	return encoded
}

// streamErrorEvent converts a terminal JSON error envelope into the error event
// of the customer's protocol.
func (w *hwMaskWriter) streamErrorEvent(data []byte) []byte {
	var envelope map[string]any
	if err := common.Unmarshal(data, &envelope); err != nil {
		return data
	}
	message, code := hwErrorEnvelopeMessage(envelope)
	if message == "" {
		message = service.HwGenericErrorMessage(w.ctx, http.StatusBadGateway)
	}
	message = hwSafetyNet(w.ctx, []byte(message))
	if w.anthropic {
		event := map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "api_error",
				"message": message,
			},
		}
		encoded, err := common.Marshal(event)
		if err != nil {
			return data
		}
		return []byte("event: error\ndata: " + string(encoded) + "\n\n")
	}
	event := map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "service_error",
			"code":    code,
		},
	}
	encoded, err := common.Marshal(event)
	if err != nil {
		return data
	}
	return []byte("data: " + string(encoded) + "\n\ndata: [DONE]\n\n")
}

// sanitizeObject applies the structural rules to one JSON object: identity and
// model names become ours, supplier-specific keys disappear, error text becomes
// our own wording.
func (w *hwMaskWriter) sanitizeObject(object map[string]any, status int) {
	for key, value := range object {
		if hwDropJSONKey(key) {
			delete(object, key)
			continue
		}
		text, ok := value.(string)
		if !ok || hwContentKeys[strings.ToLower(key)] {
			continue
		}
		if common.HwHasProviderTrace(text) {
			object[key] = hwSafetyNet(w.ctx, []byte(text))
		}
	}
	if customerModel := w.customerModel(); customerModel != "" {
		if _, ok := object["model"].(string); ok {
			object["model"] = customerModel
		}
	}
	if id, ok := object["id"].(string); ok && hwIsIdentityObject(object) {
		w.recordUpstreamID(id)
		object["id"] = w.ownID(hwIdentityPrefix(object))
	}
	for _, nestedKey := range []string{"message", "response"} {
		if inner, ok := object[nestedKey].(map[string]any); ok {
			w.sanitizeNestedIdentity(inner)
		}
	}
	if errObject, ok := object["error"].(map[string]any); ok {
		w.sanitizeErrorObject(errObject, status)
	}
}

// sanitizeNestedIdentity rewrites the identity of a nested message/response
// object (Anthropic message_start, Responses API events) without touching ids
// that belong to content blocks or tool calls.
func (w *hwMaskWriter) sanitizeNestedIdentity(inner map[string]any) {
	for key, value := range inner {
		if hwDropJSONKey(key) {
			delete(inner, key)
			continue
		}
		text, ok := value.(string)
		if !ok || hwContentKeys[strings.ToLower(key)] {
			continue
		}
		if common.HwHasProviderTrace(text) {
			inner[key] = hwSafetyNet(w.ctx, []byte(text))
		}
	}
	if customerModel := w.customerModel(); customerModel != "" {
		if _, ok := inner["model"].(string); ok {
			inner["model"] = customerModel
		}
	}
	if id, ok := inner["id"].(string); ok && hwIsIdentityObject(inner) {
		w.recordUpstreamID(id)
		inner["id"] = w.ownID("msg_hw-")
	}
}

func (w *hwMaskWriter) sanitizeErrorObject(errObject map[string]any, status int) {
	for key := range errObject {
		if hwDropJSONKey(key) {
			delete(errObject, key)
		}
	}
	code := hwStringValue(errObject["code"])
	if code == "" {
		code = hwStringValue(errObject["type"])
	}
	if !service.HwIsOwnErrorCode(code) {
		if _, ok := errObject["message"]; ok {
			errObject["message"] = service.HwGenericErrorMessage(w.ctx, status)
		}
	} else if message, ok := errObject["message"].(string); ok {
		errObject["message"] = hwSafetyNet(w.ctx, []byte(message))
	}
	for _, key := range []string{"type", "code"} {
		if text, ok := errObject[key].(string); ok && common.HwHasProviderTrace(text) {
			errObject[key] = common.HwReplaceProviderWords(text)
		}
	}
}

func (w *hwMaskWriter) customerModel() string {
	if w.ownModel != "" {
		return w.ownModel
	}
	w.ownModel = service.HwCustomerModel(w.ctx)
	return w.ownModel
}

func (w *hwMaskWriter) ownID(prefix string) string {
	if w.requestID == "" {
		w.requestID = common.NewRequestId()
	}
	return prefix + w.requestID
}

func (w *hwMaskWriter) recordUpstreamID(id string) {
	if id == "" || strings.HasPrefix(id, "hw-") || strings.HasPrefix(id, "msg_hw-") || strings.HasPrefix(id, "resp_hw-") {
		return
	}
	service.HwNoteUpstreamIdentity(w.ctx, id)
}

// hwIdentityPrefix picks the id prefix that matches the object being rewritten.
func hwIdentityPrefix(object map[string]any) string {
	if objectType := hwStringValue(object["type"]); strings.HasPrefix(objectType, "message") {
		return "msg_hw-"
	}
	if objectType := hwStringValue(object["object"]); objectType == "response" {
		return "resp_hw-"
	}
	return "hw-"
}

// hwIsIdentityObject reports whether an object carries the protocol identity we
// must rewrite, so ids of tool calls and content blocks stay untouched.
func hwIsIdentityObject(object map[string]any) bool {
	for _, key := range []string{"object", "created", "choices", "type", "role", "model", "usage"} {
		if _, ok := object[key]; ok {
			return true
		}
	}
	return false
}

func hwDropJSONKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	switch lower {
	case "provider", "system_fingerprint", "native_finish_reason", "upstream_id", "upstream_model",
		"provider_name", "provider_slug", "generation_id":
		return true
	}
	return strings.HasPrefix(lower, "openrouter") || strings.HasPrefix(lower, "x_") || strings.HasPrefix(lower, "x-")
}

// hwResponseHeaderAllowed keeps headers our own middleware set and a small
// allowlist; everything that smells like a supplier or proxy is dropped.
func hwResponseHeaderAllowed(name string, base map[string]bool) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if common.HwHasProviderTrace(lower) || strings.HasPrefix(lower, "cf-") || lower == "server" || lower == "via" {
		return false
	}
	if base[lower] {
		return true
	}
	if hwAllowedResponseHeaders[lower] {
		return true
	}
	return strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "ratelimit-")
}

// hwSafetyNet is the last-resort scan of text that is about to reach the
// customer. A hit is reported to the admin log without its content.
func hwSafetyNet(ctx *gin.Context, data []byte) string {
	text := string(data)
	if !hwProviderNetEnabled.Load() || !common.HwHasProviderTrace(text) {
		return text
	}
	service.HwNoteMaskAlert(ctx)
	return common.HwReplaceProviderWords(text)
}

// hwLooksLikeSSE reports whether a stream write already uses SSE syntax.
func hwLooksLikeSSE(data []byte) bool {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return true
	}
	switch trimmed[0] {
	case ':':
		return true
	case 'd':
		return bytes.HasPrefix(trimmed, []byte("data:"))
	case 'e':
		return bytes.HasPrefix(trimmed, []byte("event:"))
	case 'i':
		return bytes.HasPrefix(trimmed, []byte("id:"))
	case 'r':
		return bytes.HasPrefix(trimmed, []byte("retry:"))
	}
	return false
}

// hwErrorEnvelopeMessage reads the code and message of a JSON error envelope.
// Only our own error text is reused; upstream wording never is.
func hwErrorEnvelopeMessage(envelope map[string]any) (string, string) {
	errObject, ok := envelope["error"].(map[string]any)
	if !ok {
		return "", ""
	}
	code := hwStringValue(errObject["code"])
	if !service.HwIsOwnErrorCode(code) {
		return "", code
	}
	return hwStringValue(errObject["message"]), code
}

func hwStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	}
	return ""
}

// hwJSONValueStart returns the offset of the first '{' or '[' in data, or -1.
func hwJSONValueStart(data []byte) int {
	for index, char := range data {
		if char == '{' || char == '[' {
			return index
		}
	}
	return -1
}

// hwJSONValueEnd returns the offset just past the JSON value starting at data[0]
// and whether the value is complete.
func hwJSONValueEnd(data []byte) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	depth := 0
	inString := false
	escaped := false
	for index := 0; index < len(data); index++ {
		char := data[index]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			switch char {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
	}
	return 0, false
}
