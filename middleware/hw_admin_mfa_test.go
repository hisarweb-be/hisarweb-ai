package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func hwMFAContext(role, id int) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/", nil)
	c.Set("role", role)
	c.Set("id", id)
	return c, recorder
}

// G10: off by default; when on, only an admin without 2FA or passkey is refused.
func TestHwAdminMFAGate(t *testing.T) {
	previous := hwAdminHasMFA
	t.Cleanup(func() { hwAdminHasMFA = previous })
	enrolled := map[int]bool{7: true}
	hwAdminHasMFA = func(userID int) bool { return enrolled[userID] }

	c, _ := hwMFAContext(common.RoleAdminUser, 1)
	assert.True(t, hwAdminMFAGate(c), "default: no change for administrators")

	t.Setenv(hwAdminMFAEnv, "true")

	c, rec := hwMFAContext(common.RoleAdminUser, 1)
	assert.False(t, hwAdminMFAGate(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "MFA_ENROLLMENT_REQUIRED")
	assert.True(t, c.IsAborted())

	c, rec = hwMFAContext(common.RoleRootUser, 2)
	assert.False(t, hwAdminMFAGate(c), "root is an administrator too")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	c, _ = hwMFAContext(common.RoleAdminUser, 7)
	assert.True(t, hwAdminMFAGate(c), "enrolled administrator passes")

	c, _ = hwMFAContext(common.RoleCommonUser, 1)
	assert.True(t, hwAdminMFAGate(c), "ordinary users are never gated")
}
