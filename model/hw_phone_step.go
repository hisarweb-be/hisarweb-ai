package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm/clause"
)

// Telegram accounts are phone-bound upstream: Telegram issues one account per
// phone number and re-registering the number retires the old account. oauth
// tokens do not carry the phone number itself, so the stable Telegram identity
// is what this fork records as the phone signal. Two accounts that bind the
// same Telegram account therefore share a phone hash, and part A's unique
// free-credit claim refuses the second one.

// HwRecordTelegramPhoneStep stores the Telegram identity of a user as the
// phone-bound signal (kind phone). The value is prefixed before hashing so it
// can never collide with a normalised +90 phone number, and like every signal
// only the keyed hash is stored.
func HwRecordTelegramPhoneStep(userID int, telegramID string) error {
	telegramID = strings.TrimSpace(telegramID)
	if userID <= 0 || telegramID == "" {
		return ErrHwInvalidUserSignal
	}
	return hwUpsertHashedSignalRow(userID, HwSignalKindPhone, "telegram:"+telegramID)
}

// HwRecordDevPhoneStep is the local-development stand-in for the Telegram step:
// it is called only when HW_SKIP_PHONE_STEP is active outside production, so
// the smoke-test registration can reach the free credit without Telegram. The
// per-user value keeps the unique claim row meaningful (one claim per user)
// while part A's shared-signal checks keep running exactly as in production.
func HwRecordDevPhoneStep(userID int) error {
	if userID <= 0 {
		return ErrHwInvalidUserSignal
	}
	return hwUpsertHashedSignalRow(userID, HwSignalKindPhone, "dev-phone:"+strconv.Itoa(userID))
}

// hwUpsertHashedSignalRow hashes an already-normalised signal value and stores
// it as one history row, with the same conflict rule as UpsertHwUserSignals:
// a repeated value only moves last_seen.
func hwUpsertHashedSignalRow(userID int, kind, value string) error {
	hash, err := HwHashSignalValue(value)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	row := HwUserSignal{UserId: userID, Kind: kind, Hash: hash, FirstSeen: now, LastSeen: now}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen"}),
	}).Create(&row).Error
}
