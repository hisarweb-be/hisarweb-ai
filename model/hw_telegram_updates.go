package model

import (
	"errors"

	"gorm.io/gorm/clause"
)

// ErrHwInvalidTelegramUpdate rejects an update id that cannot come from the
// Telegram bot API: Telegram numbers its updates from one upwards.
var ErrHwInvalidTelegramUpdate = errors.New("invalid telegram update id")

// HwTelegramUpdate records a Telegram update that was already handled. Telegram
// delivers an update again when the webhook answer arrives late, and the same
// update may be retried after a timeout, so handling must be claimed once. The
// table holds the update id only, never a message body, a phone number or a
// chat id.
type HwTelegramUpdate struct {
	UpdateId  int64 `json:"update_id" gorm:"primaryKey;autoIncrement:false"`
	CreatedAt int64 `json:"created_at" gorm:"not null;default:0"`
}

func (HwTelegramUpdate) TableName() string {
	return "hw_telegram_updates"
}

// HwClaimTelegramUpdate claims one update id for handling. A second claim of the
// same id returns false, which lets the caller answer the repeated delivery
// without processing it again. The claim is taken before the work starts so two
// deliveries racing each other cannot both process the update.
func HwClaimTelegramUpdate(updateID int64, now int64) (bool, error) {
	if updateID <= 0 {
		return false, ErrHwInvalidTelegramUpdate
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&HwTelegramUpdate{UpdateId: updateID, CreatedAt: now})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// HwReleaseTelegramUpdate gives back a claim whose handling failed, so the
// retry Telegram sends for a non-2xx answer can still do the work. Every write
// of the handler is idempotent, but a released claim is what keeps a transient
// database failure from dropping the customer's only contact message.
func HwReleaseTelegramUpdate(updateID int64) error {
	if updateID <= 0 {
		return ErrHwInvalidTelegramUpdate
	}
	return DB.Where("update_id = ?", updateID).Delete(&HwTelegramUpdate{}).Error
}

// HwTelegramUpdateKeepDays is how long a claim is kept. Telegram retries a
// delivery for a short while only, so a week covers every retry it makes and
// keeps the table from growing one row per onboarding message forever.
const HwTelegramUpdateKeepDays = 7

// HwDeleteOldTelegramUpdates drops the claims older than keepDays, counted back
// from now. The daily abuse sweep calls it; a claim that is gone can only be
// claimed again, which is the idempotent path the handler already takes.
func HwDeleteOldTelegramUpdates(now int64, keepDays int) error {
	if keepDays <= 0 {
		keepDays = HwTelegramUpdateKeepDays
	}
	return DB.Where("created_at < ?", now-int64(keepDays)*24*3600).Delete(&HwTelegramUpdate{}).Error
}
