package common

// HwVerificationMaxAttempts is the number of wrong guesses a code survives
// (ASVS V2.2.1/V2.7: a 6-character code must not be brute-forceable within its
// lifetime). After the last allowed wrong guess the code is deleted, so even the
// right code fails and the user has to request a fresh one. The counter is per
// process, like the memory map. Callers hold verificationMutex.
const HwVerificationMaxAttempts = 5

var verificationAttempts = map[string]int{}

func hwVerificationResetAttempts(purpose, key string) {
	delete(verificationAttempts, purpose+key)
}

// hwVerificationCheck compares the guess with the stored digest of a live code
// and counts wrong guesses against it.
func hwVerificationCheck(purpose, key, code, stored string) bool {
	id := purpose + key
	if hwVerificationDigestsEqual(stored, hwVerificationDigest(purpose, key, code)) {
		delete(verificationAttempts, id)
		return true
	}
	verificationAttempts[id]++
	if verificationAttempts[id] >= HwVerificationMaxAttempts {
		delete(verificationAttempts, id)
		delete(verificationMap, id)
		hwVerificationRemoteDelete(purpose, key)
	}
	return false
}
