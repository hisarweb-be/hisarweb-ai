package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// hwSignalSaltEnv names the server-side secret used to hash phone numbers, IP
// addresses and card fingerprints before they are stored. There is deliberately
// no default salt in code: without a secret nothing can be hashed.
const hwSignalSaltEnv = "HW_SIGNAL_SALT"

// ErrHwSignalSaltMissing is returned when HW_SIGNAL_SALT is not configured.
// There is no fallback: the hash of a phone number, IP address or card
// fingerprint must be keyed with a secret that is dedicated to this purpose,
// because the server crypto secret can be rotated for other reasons and would
// silently change every stored hash. Callers must treat the error as "do not
// store the signal".
var ErrHwSignalSaltMissing = errors.New("HW_SIGNAL_SALT is not configured")

// ErrHwInvalidUserSignal rejects a signal write for an unknown user.
var ErrHwInvalidUserSignal = errors.New("invalid user signal")

// Signal kinds recorded in hw_user_signals. Every kind is hashed before it is
// stored; the table never holds a raw address, number or fingerprint.
const (
	HwSignalKindDevice = "device"
	HwSignalKindIp     = "ip"
	HwSignalKindPhone  = "phone"
	HwSignalKindCard   = "card"
)

// HwUserSignal is one historical abuse signal of a user. A user has many rows:
// changing an IP address or a device does not replace the earlier value, because
// a later account that turns up with the old value must still match. The
// (user_id, kind, hash) triple is unique, so the same value only moves LastSeen.
type HwUserSignal struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"not null;uniqueIndex:idx_hw_user_signals_identity,priority:1;index:idx_hw_user_signals_kind_hash,priority:1"`
	Kind      string `json:"kind" gorm:"type:varchar(16);not null;uniqueIndex:idx_hw_user_signals_identity,priority:2;index:idx_hw_user_signals_kind_hash,priority:2"`
	Hash      string `json:"hash" gorm:"type:varchar(64);not null;uniqueIndex:idx_hw_user_signals_identity,priority:3;index:idx_hw_user_signals_kind_hash,priority:3"`
	FirstSeen int64  `json:"first_seen" gorm:"not null;default:0"`
	LastSeen  int64  `json:"last_seen" gorm:"not null;default:0"`
}

func (HwUserSignal) TableName() string {
	return "hw_user_signals"
}

// HwUserSignals is a patch of abuse signals, with one clearly named field per
// kind. Every field carries the RAW value: the device token as the browser sent
// it, the client IP address as a string, the phone number as the customer typed
// it, and only the fingerprint of a card as the payment provider derived it
// (never the card number itself). UpsertHwUserSignals hashes them all; nothing
// here is stored as given, and empty fields are ignored instead of erasing
// history.
//
// A value that happens to look like one of our own 64-character hex hashes is
// still treated as a raw value and hashed again. Recognising our own hashes
// would only be possible by keeping something to compare against, which is
// exactly the data this table must not hold.
type HwUserSignals struct {
	DeviceToken     string `json:"device_token"`
	Ip              string `json:"ip"`
	Phone           string `json:"phone"`
	CardFingerprint string `json:"card_fingerprint"`
}

// HwSignalSalt resolves the key used by HwHashSignalValue. HW_SIGNAL_SALT is
// the only source; without it hashing is refused with ErrHwSignalSaltMissing
// instead of falling back to the server crypto secret.
func HwSignalSalt() (string, error) {
	if salt := strings.TrimSpace(os.Getenv(hwSignalSaltEnv)); salt != "" {
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

// UpsertHwUserSignals hashes the non-empty raw values of a patch and stores them
// as history rows. A raw IP address, phone number or card fingerprint never
// reaches the table or a log line.
func UpsertHwUserSignals(userID int, patch HwUserSignals) error {
	if userID <= 0 {
		return ErrHwInvalidUserSignal
	}
	now := common.GetTimestamp()
	rows := make([]HwUserSignal, 0, 4)
	for _, signal := range []struct {
		kind  string
		value string
	}{
		{HwSignalKindDevice, patch.DeviceToken},
		{HwSignalKindIp, patch.Ip},
		{HwSignalKindPhone, patch.Phone},
		{HwSignalKindCard, patch.CardFingerprint},
	} {
		if strings.TrimSpace(signal.value) == "" {
			continue
		}
		hash, err := HwHashSignalValue(signal.value)
		if err != nil {
			return err
		}
		rows = append(rows, HwUserSignal{
			UserId:    userID,
			Kind:      signal.kind,
			Hash:      hash,
			FirstSeen: now,
			LastSeen:  now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return DB.Create(&rows).Error
}

// hwUserHasVerifiedSignal reports whether the user produced at least one device
// or phone signal. Those two kinds need a step a throwaway account cannot fake
// cheaply; an IP address or card fingerprint alone proves nothing about the
// person behind the account.
func hwUserHasVerifiedSignal(userID int) (bool, error) {
	var count int64
	err := DB.Model(&HwUserSignal{}).
		Where("user_id = ? AND kind IN ? AND hash <> ?", userID, []string{HwSignalKindDevice, HwSignalKindPhone}, "").
		Count(&count).Error
	return count > 0, err
}

// hwSharedSignalKinds lists the signal kinds two users have in common, over the
// full history of both. Only non-empty hashes count, so users without a recorded
// signal never match.
func hwSharedSignalKinds(a, b int) ([]string, error) {
	if a <= 0 || b <= 0 || a == b {
		return nil, nil
	}
	var signals []HwUserSignal
	if err := DB.Where("user_id IN ? AND hash <> ?", []int{a, b}, "").Find(&signals).Error; err != nil {
		return nil, err
	}
	hashes := make(map[int]map[string]map[string]struct{}, 2)
	for _, signal := range signals {
		if hashes[signal.UserId] == nil {
			hashes[signal.UserId] = make(map[string]map[string]struct{}, 4)
		}
		if hashes[signal.UserId][signal.Kind] == nil {
			hashes[signal.UserId][signal.Kind] = make(map[string]struct{})
		}
		hashes[signal.UserId][signal.Kind][signal.Hash] = struct{}{}
	}
	kinds := make([]string, 0, 4)
	for _, kind := range []string{HwSignalKindDevice, HwSignalKindIp, HwSignalKindPhone, HwSignalKindCard} {
		for hash := range hashes[a][kind] {
			if _, ok := hashes[b][kind][hash]; ok {
				kinds = append(kinds, kind)
				break
			}
		}
	}
	return kinds, nil
}

// HwSharedSignal reports whether two users share a device, IP address, phone
// number or card fingerprint, now or at any point in the past.
func HwSharedSignal(a, b int) (bool, error) {
	kinds, err := hwSharedSignalKinds(a, b)
	if err != nil {
		return false, err
	}
	return len(kinds) > 0, nil
}
