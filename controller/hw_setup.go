package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	// HwSetupTokenEnv holds the one-time secret that opens the first-run setup
	// of a production deployment. It only lives in the environment of the
	// deployment platform, never in the database, the code or a log line.
	HwSetupTokenEnv = "HW_SETUP_TOKEN"
	// HwSetupTokenHeader is the request header that carries that secret.
	HwSetupTokenHeader = "X-HW-Setup-Token"
)

var (
	// hwSetupMu serialises the first-run setup. Two requests that both see an
	// empty database would otherwise each create an administrator; the second
	// one now waits and then finds the system initialized.
	hwSetupMu sync.Mutex

	errHwSetupTokenMissing = errors.New("setup is closed: this deployment is in production and HW_SETUP_TOKEN is not configured")
	errHwSetupTokenInvalid = errors.New("setup refused: the setup token is missing or wrong")
)

// HwAuthorizeSetupToken decides whether POST /api/setup may run. Outside
// production it always may, so a laptop run keeps the plain setup page. In
// production the request must carry the token of HW_SETUP_TOKEN, and a
// deployment that configured no token refuses every attempt: an open setup
// endpoint lets whoever reaches the server first become its administrator.
// Both sides are hashed before the comparison so the check takes the same time
// whatever their length.
func HwAuthorizeSetupToken(c *gin.Context) error {
	if !common.HwIsProduction() {
		return nil
	}
	want := strings.TrimSpace(os.Getenv(HwSetupTokenEnv))
	if want == "" {
		return errHwSetupTokenMissing
	}
	got := strings.TrimSpace(c.GetHeader(HwSetupTokenHeader))
	gotSum := sha256.Sum256([]byte(got))
	wantSum := sha256.Sum256([]byte(want))
	if subtle.ConstantTimeCompare(gotSum[:], wantSum[:]) != 1 {
		return errHwSetupTokenInvalid
	}
	return nil
}

// hwRefuseSetup answers a refused setup request. The status is 403 and the
// message names the missing secret without ever echoing the value: an operator
// sees what to configure, a stranger learns nothing to guess.
func hwRefuseSetup(c *gin.Context, err error) {
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": err.Error(),
	})
}
