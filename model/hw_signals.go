package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// hwSignalSaltEnv names the server-side secret used to hash phone numbers, IP
// addresses and card fingerprints before they are stored. There is deliberately
// no default salt in code: without a secret nothing can be hashed.
const hwSignalSaltEnv = "HW_SIGNAL_SALT"

// ErrHwSignalSaltMissing is returned when neither HW_SIGNAL_SALT nor the server
// crypto secret is available. Callers must treat it as "do not store the signal".
var ErrHwSignalSaltMissing = errors.New("HW_SIGNAL_SALT is not configured")

// ErrHwInvalidUserSignal rejects a signal write for an unknown user.
var ErrHwInvalidUserSignal = errors.New("invalid user signal")

// HwUserSignal holds the abuse signals of a user. Values that identify a person
// (IP address, phone number, card) are stored only as keyed hashes.
type HwUserSignal struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	UserId          int    `json:"user_id" gorm:"not null;uniqueIndex"`
	DeviceToken     string `json:"device_token" gorm:"type:varchar(128);index"`
	IpHash          string `json:"ip_hash" gorm:"type:varchar(64);index"`
	PhoneHash       string `json:"phone_hash" gorm:"type:varchar(64);index"`
	CardFingerprint string `json:"card_fingerprint" gorm:"type:varchar(128);index"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

func (HwUserSignal) TableName() string {
	return "hw_user_signals"
}

// HwUserSignals is a patch of abuse signals. Empty fields keep the stored value.
type HwUserSignals struct {
	DeviceToken     string `json:"device_token"`
	IpHash          string `json:"ip_hash"`
	PhoneHash       string `json:"phone_hash"`
	CardFingerprint string `json:"card_fingerprint"`
}

// HwSignalSalt resolves the key used by HwHashSignalValue. HW_SIGNAL_SALT wins;
// otherwise the server crypto secret (derived from SESSION_SECRET/CRYPTO_SECRET)
// is used, so hashes stay comparable across restarts and nodes.
func HwSignalSalt() (string, error) {
	if salt := strings.TrimSpace(os.Getenv(hwSignalSaltEnv)); salt != "" {
		return salt, nil
	}
	if salt := strings.TrimSpace(common.CryptoSecret); salt != "" {
		return salt, nil
	}
	return "", ErrHwSignalSaltMissing
}

// HwHashSignalValue returns the keyed hash stored for a raw IP address, phone
// number or card fingerprint. Callers must not store the raw value instead.
func HwHashSignalValue(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	salt, err := HwSignalSalt()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// UpsertHwUserSignals stores the non-empty signals of a patch and leaves the
// other columns untouched.
func UpsertHwUserSignals(userID int, patch HwUserSignals) error {
	if userID <= 0 {
		return ErrHwInvalidUserSignal
	}
	updates := map[string]any{}
	if patch.DeviceToken != "" {
		updates["device_token"] = patch.DeviceToken
	}
	if patch.IpHash != "" {
		updates["ip_hash"] = patch.IpHash
	}
	if patch.PhoneHash != "" {
		updates["phone_hash"] = patch.PhoneHash
	}
	if patch.CardFingerprint != "" {
		updates["card_fingerprint"] = patch.CardFingerprint
	}
	if len(updates) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	updates["updated_at"] = now

	var existing HwUserSignal
	err := DB.Where("user_id = ?", userID).First(&existing).Error
	switch {
	case err == nil:
		return DB.Model(&HwUserSignal{}).Where("user_id = ?", userID).Updates(updates).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return DB.Create(&HwUserSignal{
			UserId:          userID,
			DeviceToken:     patch.DeviceToken,
			IpHash:          patch.IpHash,
			PhoneHash:       patch.PhoneHash,
			CardFingerprint: patch.CardFingerprint,
			CreatedAt:       now,
			UpdatedAt:       now,
		}).Error
	default:
		return err
	}
}

// hwSharedSignalKinds lists the signals two users have in common. Only non-empty
// values count, so users without a recorded signal never match.
func hwSharedSignalKinds(a, b int) ([]string, error) {
	if a <= 0 || b <= 0 || a == b {
		return nil, nil
	}
	var signals []HwUserSignal
	if err := DB.Where("user_id IN ?", []int{a, b}).Find(&signals).Error; err != nil {
		return nil, err
	}
	byUser := make(map[int]HwUserSignal, len(signals))
	for _, signal := range signals {
		byUser[signal.UserId] = signal
	}
	signalA, okA := byUser[a]
	signalB, okB := byUser[b]
	if !okA || !okB {
		return nil, nil
	}
	kinds := make([]string, 0, 4)
	if hwSignalValueShared(signalA.DeviceToken, signalB.DeviceToken) {
		kinds = append(kinds, "device")
	}
	if hwSignalValueShared(signalA.IpHash, signalB.IpHash) {
		kinds = append(kinds, "ip")
	}
	if hwSignalValueShared(signalA.PhoneHash, signalB.PhoneHash) {
		kinds = append(kinds, "phone")
	}
	if hwSignalValueShared(signalA.CardFingerprint, signalB.CardFingerprint) {
		kinds = append(kinds, "card")
	}
	return kinds, nil
}

func hwSignalValueShared(a, b string) bool {
	return a != "" && a == b
}

// HwSharedSignal reports whether two users share a device, IP hash, phone hash
// or card fingerprint.
func HwSharedSignal(a, b int) (bool, error) {
	kinds, err := hwSharedSignalKinds(a, b)
	if err != nil {
		return false, err
	}
	return len(kinds) > 0, nil
}
