package common

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type verificationValue struct {
	code string
	time time.Time
}

const (
	EmailVerificationPurpose = "v"
	PasswordResetPurpose     = "r"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10
var VerificationValidMinutes = 10

func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	digest := hwVerificationDigest(purpose, key, code)
	hwVerificationRemoteStore(purpose, key, digest)
	hwVerificationResetAttempts(purpose, key)
	verificationMap[purpose+key] = verificationValue{
		code: digest,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

func VerifyCodeWithKey(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	stored := ""
	if digest, found := hwVerificationRemoteLoad(purpose, key); found {
		stored = digest
	} else {
		value, okay := verificationMap[purpose+key]
		if !okay || int(time.Since(value.time).Seconds()) >= VerificationValidMinutes*60 {
			return false
		}
		stored = value.code
	}
	return hwVerificationCheck(purpose, key, code, stored)
}

func DeleteKey(key string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	hwVerificationRemoteDelete(purpose, key)
	delete(verificationMap, purpose+key)
	hwVerificationResetAttempts(purpose, key)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
			delete(verificationAttempts, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
