package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm/clause"
)

// The Telegram login widget returns the account identity only: id, name, user
// name, photo, auth date and the signature. The login widget never carries the
// phone number, and a person who retires a Telegram account can start another
// one with the same SIM, so the identity is a weak signal. The number itself
// comes from the bot's share-contact button (request_contact), which Telegram
// marks as the sender's own number. This fork therefore records the identity
// under its own kind and trusts only the shared number as the phone signal.

// HwRecordTelegramPhoneStep stores the Telegram identity of a user as the weak
// Telegram signal (kind telegram), never as a phone signal. The value is
// prefixed before hashing so it can never collide with a normalised +90 phone
// number, and like every signal only the keyed hash is stored.
func HwRecordTelegramPhoneStep(userID int, telegramID string) error {
	telegramID = strings.TrimSpace(telegramID)
	if userID <= 0 || telegramID == "" {
		return ErrHwInvalidUserSignal
	}
	return hwUpsertHashedSignalRow(userID, HwSignalKindTelegram, "telegram:"+telegramID)
}

// HwRecordPhoneSignal normalises a number that the customer shared through the
// bot's contact button and stores its keyed hash as the phone signal (kind
// phone). It is the only path that writes a phone signal, so the free trial
// credit can only follow a number Telegram itself attributes to the sender.
// The raw number never reaches the table, a log line or an answer; an invalid
// or foreign number is refused with ErrHwInvalidPhoneNumber and stores nothing.
func HwRecordPhoneSignal(userID int, rawPhone string) error {
	if userID <= 0 {
		return ErrHwInvalidUserSignal
	}
	phone, err := hwNormalizePhone(rawPhone)
	if err != nil {
		return err
	}
	return hwUpsertHashedSignalRow(userID, HwSignalKindPhone, phone)
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
