package router

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The admin agent is proven where it is reached: over HTTP, through the real
// session authentication. A customer and a visitor never get the chat, the
// text editor or the escalation card; only the read of the applied texts is
// public (the customer pages need it).

type hwAgentRoute struct {
	method string
	path   string
	body   string
}

var hwAgentAdminRoutes = []hwAgentRoute{
	{http.MethodPost, "/api/hw/admin/agent/chat", `{"messages":[{"role":"user","content":"hallo"}]}`},
	{http.MethodGet, "/api/hw/admin/agent/texts", ``},
	{http.MethodPost, "/api/hw/admin/agent/texts", `{"lang":"tr","key":"Save","value":"Kaydet"}`},
	{http.MethodPost, "/api/hw/admin/agent/texts/revert", `{"lang":"tr","key":"Save"}`},
	{http.MethodPost, "/api/hw/admin/agent/escalate", `{"title":"t","summary":"s"}`},
	{http.MethodGet, "/api/hw/admin/agent/escalations", ``},
	{http.MethodPost, "/api/hw/admin/agent/escalations/card", `{"id":1,"card":"HIS-1"}`},
}

func hwAgentEnv(t *testing.T) (*hwReserveTestEnv, *model.User, *model.User) {
	t.Helper()
	env := hwReserveTestCase(t)
	require.NoError(t, model.DB.AutoMigrate(&model.HwTextOverride{}, &model.HwAdminEscalation{}))
	env.engine.Use(middleware.RequestId())
	SetHwRouter(env.engine.Group("/api"))
	customer, _ := env.customer("default")
	admin, _ := env.customer("default")
	require.NoError(t, model.DB.Model(admin).Update("role", common.RoleAdminUser).Error)
	admin.Role = common.RoleAdminUser
	return env, customer, admin
}

func (env *hwReserveTestEnv) agentCall(route hwAgentRoute, bearer string) *httptest.ResponseRecorder {
	env.t.Helper()
	request := httptest.NewRequest(route.method, route.path, bytes.NewReader([]byte(route.body)))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	env.engine.ServeHTTP(recorder, request)
	return recorder
}

func TestHwAdminAgentRoutesRefuseVisitorsAndCustomers(t *testing.T) {
	env, customer, _ := hwAgentEnv(t)
	customerBearer := env.bearer(customer)
	for _, route := range hwAgentAdminRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			visitor := env.agentCall(route, "")
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, visitor.Code, "visitor: %s", visitor.Body.String())
			assert.NotContains(t, visitor.Body.String(), `"success":true`)

			asCustomer := env.agentCall(route, customerBearer)
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, asCustomer.Code, "customer: %s", asCustomer.Body.String())
			assert.NotContains(t, asCustomer.Body.String(), `"success":true`)
		})
	}
	var stored int64
	require.NoError(t, model.DB.Model(&model.HwTextOverride{}).Count(&stored).Error)
	assert.Zero(t, stored, "a refused request must not store a text")
}

func TestHwAdminAgentAdminAppliesAndPublicReadsText(t *testing.T) {
	env, _, admin := hwAgentEnv(t)
	adminBearer := env.bearer(admin)

	applied := env.agentCall(hwAgentRoute{http.MethodPost, "/api/hw/admin/agent/texts", `{"lang":"tr","key":"Hello {{name}}","value":"Merhaba {{name}}"}`}, adminBearer)
	require.Equal(t, http.StatusOK, applied.Code, applied.Body.String())
	assert.Contains(t, applied.Body.String(), `"success":true`)

	// The customer pages read the texts without a login.
	public := env.agentCall(hwAgentRoute{http.MethodGet, "/api/hw/text-overrides?lang=tr", ``}, "")
	require.Equal(t, http.StatusOK, public.Code, public.Body.String())
	assert.Contains(t, public.Body.String(), "Merhaba {{name}}")

	bad := env.agentCall(hwAgentRoute{http.MethodPost, "/api/hw/admin/agent/texts", `{"lang":"tr","key":"Hello {{name}}","value":"Merhaba"}`}, adminBearer)
	assert.Contains(t, bad.Body.String(), `"success":false`, "a text that drops a placeholder is refused")

	reverted := env.agentCall(hwAgentRoute{http.MethodPost, "/api/hw/admin/agent/texts/revert", `{"lang":"tr","key":"Hello {{name}}"}`}, adminBearer)
	require.Equal(t, http.StatusOK, reverted.Code, reverted.Body.String())
	after := env.agentCall(hwAgentRoute{http.MethodGet, "/api/hw/text-overrides?lang=tr", ``}, "")
	assert.NotContains(t, after.Body.String(), "Merhaba")
}

func TestHwAdminAgentChatReachesTheGatewayForAnAdmin(t *testing.T) {
	var gotAuth, gotModel string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var request struct {
			Model string `json:"model"`
		}
		_ = common.DecodeJson(r.Body, &request)
		gotModel = request.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"reply\":\"Beter: Kaydet\",\"text_fix\":{\"lang\":\"tr\",\"shown\":\"Sakla\",\"proposed\":\"Kaydet\"},\"escalate\":null}"}}]}`))
	}))
	t.Cleanup(gateway.Close)
	t.Setenv("HW_ADMIN_AGENT_BASE_URL", gateway.URL)
	t.Setenv("HW_ADMIN_AGENT_API_KEY", "agent-test-key")
	t.Setenv("HW_ADMIN_AGENT_MODEL", "")

	env, _, admin := hwAgentEnv(t)
	response := env.agentCall(hwAgentAdminRoutes[0], env.bearer(admin))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "Beter: Kaydet")
	assert.Contains(t, response.Body.String(), `"proposed":"Kaydet"`)
	assert.Equal(t, "Bearer agent-test-key", gotAuth)
	assert.Equal(t, "claude-haiku-5-5", gotModel, "the cheapest capable model is the default")
}

func TestHwAdminAgentEscalationQueuesWhenBoardIsUnreachable(t *testing.T) {
	env, _, admin := hwAgentEnv(t)
	adminBearer := env.bearer(admin)
	t.Setenv("HW_BUREAU_URL", "")

	queued := env.agentCall(hwAgentRoute{http.MethodPost, "/api/hw/admin/agent/escalate", `{"title":"Betaling mislukt","summary":"500 TRY geeft een fout","page":"/console/topup"}`}, adminBearer)
	require.Equal(t, http.StatusOK, queued.Code, queued.Body.String())
	assert.Contains(t, queued.Body.String(), `"success":true`)
	assert.Contains(t, queued.Body.String(), `"queued":true`)

	pending := env.agentCall(hwAgentRoute{http.MethodGet, "/api/hw/admin/agent/escalations", ``}, adminBearer)
	assert.Contains(t, pending.Body.String(), "Betaling mislukt")

	linked := env.agentCall(hwAgentRoute{http.MethodPost, "/api/hw/admin/agent/escalations/card", `{"id":1,"card":"HIS-7"}`}, adminBearer)
	assert.Contains(t, linked.Body.String(), `"success":true`)
	after := env.agentCall(hwAgentRoute{http.MethodGet, "/api/hw/admin/agent/escalations", ``}, adminBearer)
	assert.NotContains(t, after.Body.String(), "Betaling mislukt", "a linked escalation is no longer pending")
}
