package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrHwInvalidTelegramUpdate rejects an update id that cannot come from the
// Telegram bot API: Telegram numbers its updates from one upwards.
var ErrHwInvalidTelegramUpdate = errors.New("invalid telegram update id")

// HwTelegramUpdateClaimStaleSeconds is how long a claim may stay unfinished
// before a later delivery takes it over. Handling one update costs a few
// database writes and at most one bot answer with a ten second timeout, so a
// claim older than two minutes belongs to a request that was killed before it
// could answer; nobody is left to give it back.
const HwTelegramUpdateClaimStaleSeconds int64 = 120

// HwTelegramUpdate records a Telegram update that is being handled or was
// already handled. Telegram delivers an update again when the webhook answer
// arrives late, and the same update may be retried after a timeout, so handling
// must be claimed once. The table holds the update id and two times only, never
// a message body, a phone number or a chat id.
type HwTelegramUpdate struct {
	UpdateId  int64 `json:"update_id" gorm:"primaryKey;autoIncrement:false"`
	CreatedAt int64 `json:"created_at" gorm:"not null;default:0"`
	// FinishedAt is zero while the claiming delivery is still running and the
	// time the work ended once it is done. A repeated delivery reads it to tell
	// "already handled" (answer 200) from "still running" (answer 503, retry).
	FinishedAt int64 `json:"finished_at" gorm:"not null;default:0"`
}

func (HwTelegramUpdate) TableName() string {
	return "hw_telegram_updates"
}

// HwTelegramUpdateClaim is what a delivery learnt about an update id.
type HwTelegramUpdateClaim int

const (
	// HwTelegramUpdateClaimed: this delivery owns the work for the update.
	HwTelegramUpdateClaimed HwTelegramUpdateClaim = iota
	// HwTelegramUpdateFinished: an earlier delivery handled the update, so a
	// repeat of it must be acknowledged without doing the work again.
	HwTelegramUpdateFinished
	// HwTelegramUpdateRunning: another delivery holds the claim and has not
	// finished. This delivery must not do the work and must not answer 200,
	// because the holder may still fail and only a retry can then save the
	// message.
	HwTelegramUpdateRunning
)

// HwClaimTelegramUpdate claims one update id for handling. The claim is taken
// before the work starts so two deliveries racing each other cannot both
// process the update, and it is finished by HwFinishTelegramUpdate or given
// back by HwReleaseTelegramUpdate.
func HwClaimTelegramUpdate(updateID int64, now int64) (HwTelegramUpdateClaim, error) {
	if updateID <= 0 {
		return HwTelegramUpdateClaimed, ErrHwInvalidTelegramUpdate
	}
	result := DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&HwTelegramUpdate{UpdateId: updateID, CreatedAt: now})
	if result.Error != nil {
		return HwTelegramUpdateClaimed, result.Error
	}
	if result.RowsAffected > 0 {
		return HwTelegramUpdateClaimed, nil
	}
	var claim HwTelegramUpdate
	if err := DB.Where("update_id = ?", updateID).First(&claim).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The holder gave the claim back between the insert and this read.
			// Nothing was handled, so the caller answers "retry".
			return HwTelegramUpdateRunning, nil
		}
		return HwTelegramUpdateClaimed, err
	}
	if claim.FinishedAt > 0 {
		return HwTelegramUpdateFinished, nil
	}
	if claim.CreatedAt > now-HwTelegramUpdateClaimStaleSeconds {
		return HwTelegramUpdateRunning, nil
	}
	// The delivery that held this claim is gone. Taking it over is guarded by the
	// start time the claim had, so of two retries only one can take it.
	result = DB.Model(&HwTelegramUpdate{}).
		Where("update_id = ? AND finished_at = 0 AND created_at = ?", updateID, claim.CreatedAt).
		Update("created_at", now)
	if result.Error != nil {
		return HwTelegramUpdateClaimed, result.Error
	}
	if result.RowsAffected != 1 {
		return HwTelegramUpdateRunning, nil
	}
	return HwTelegramUpdateClaimed, nil
}

// HwFinishTelegramUpdate records that the work of the claimed update is done, so
// every later delivery of it is acknowledged instead of handled. startedAt is
// the time handed to HwClaimTelegramUpdate: a stale claim another delivery has
// taken over carries a newer start time, and the older delivery leaves it alone.
func HwFinishTelegramUpdate(updateID int64, startedAt int64, finishedAt int64) error {
	if updateID <= 0 {
		return ErrHwInvalidTelegramUpdate
	}
	return DB.Model(&HwTelegramUpdate{}).
		Where("update_id = ? AND created_at = ?", updateID, startedAt).
		Update("finished_at", finishedAt).Error
}

// HwReleaseTelegramUpdate gives back a claim whose handling failed, so the
// retry Telegram sends for a non-2xx answer can still do the work. Every write
// of the handler is idempotent, but a released claim is what keeps a transient
// database failure from dropping the customer's only contact message. As in
// HwFinishTelegramUpdate, startedAt keeps a taken-over claim out of reach.
func HwReleaseTelegramUpdate(updateID int64, startedAt int64) error {
	if updateID <= 0 {
		return ErrHwInvalidTelegramUpdate
	}
	return DB.Where("update_id = ? AND created_at = ?", updateID, startedAt).
		Delete(&HwTelegramUpdate{}).Error
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
