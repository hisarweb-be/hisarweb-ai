package common

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// PwnedPasswordsRangeURL is the k-anonymity range endpoint of Pwned Passwords
// (free, no key). Only the first 5 hex characters of the SHA-1 leave the
// server. An empty value turns the check off.
const defaultPwnedURL = "https://api.pwnedpasswords.com/range/"

var PwnedPasswordsRangeURL = defaultPwnedURL

var pwnedHTTPClient = &http.Client{Timeout: 2 * time.Second}

// isBreachedPassword fails open: if the service is unreachable the password is
// accepted and a log line records it (ASVS V2.1.7). The check runs in
// production only, so development and test binaries never call out.
func isBreachedPassword(password string) bool {
	// Test binaries never call the real service; a test that wants the check
	// points PwnedPasswordsRangeURL at its own server.
	if PwnedPasswordsRangeURL == "" || (PwnedPasswordsRangeURL == defaultPwnedURL && (testing.Testing() || !HwIsProduction())) {
		return false
	}
	sum := sha1.Sum([]byte(password))
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := full[:5], full[5:]
	req, err := http.NewRequest(http.MethodGet, PwnedPasswordsRangeURL+prefix, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Add-Padding", "true")
	resp, err := pwnedHTTPClient.Do(req)
	if err != nil {
		SysLog("breached-password check skipped: service unreachable")
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		SysLog(fmt.Sprintf("breached-password check skipped: status %d", resp.StatusCode))
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		SysLog("breached-password check skipped: read failed")
		return false
	}
	for _, line := range strings.Split(string(body), "\n") {
		hash, count, ok := strings.Cut(strings.TrimSpace(line), ":")
		// Add-Padding rows carry a count of 0 and are not real matches.
		if ok && strings.EqualFold(hash, suffix) && strings.TrimSpace(count) != "0" {
			return true
		}
	}
	return false
}
