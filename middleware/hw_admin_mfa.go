package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// HW_ADMIN_MFA_REQUIRED=true makes every admin and root request refuse until the
// account has two-factor authentication or a passkey (ASVS V4.3.1). Default off:
// switching it on without warning would lock out administrators who have not
// enrolled yet, so the operator enrols them first, then sets the variable. The
// enrolment routes live under the plain user auth, not the admin auth, so a
// locked-out administrator can still reach them and enrol.
const hwAdminMFAEnv = "HW_ADMIN_MFA_REQUIRED"

// hwAdminHasMFA is a variable so a test can answer without a database.
var hwAdminHasMFA = func(userID int) bool {
	if enabled, err := model.IsTwoFAEnabled(userID); err == nil && enabled {
		return true
	}
	_, err := model.GetPasskeyByUserID(userID)
	return err == nil && !errors.Is(err, model.ErrPasskeyNotFound)
}

// hwAdminMFAGate reports whether the request may continue. It has written the
// refusal when it returns false.
func hwAdminMFAGate(c *gin.Context) bool {
	if c.GetInt("role") < common.RoleAdminUser || !common.GetEnvOrDefaultBool(hwAdminMFAEnv, false) {
		return true
	}
	if hwAdminHasMFA(c.GetInt("id")) {
		return true
	}
	abortWithWebMessage(c, http.StatusForbidden, "MFA_ENROLLMENT_REQUIRED",
		common.NewMessage("Administrators must enable two-factor authentication or a passkey first"))
	return false
}
