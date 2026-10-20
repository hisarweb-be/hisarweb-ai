package model

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HwMaxFreeTrialDollars is the hard cap of the free trial credit in US dollars.
const HwMaxFreeTrialDollars = 10

// Reasons HwGrantFreeTrialCredit refuses a grant with.
const (
	HwFreeTrialUnverified     = "unverified"
	HwFreeTrialAlreadyGranted = "already_granted"
	HwFreeTrialQuotaTooHigh   = "quota_too_high"
	HwFreeTrialSharedSignal   = "shared_"
)

// HwMaxFreeTrialQuota is the hard upper bound of the free trial credit in quota
// units: HwMaxFreeTrialDollars at the configured rate. It is a function instead
// of a constant because common.QuotaPerUnit is configuration, not a compile-time
// value.
func HwMaxFreeTrialQuota() int {
	return common.QuotaFromFloat(HwMaxFreeTrialDollars * common.QuotaPerUnit)
}

// HwFreeCreditClaim records which phone hash already received the free trial
// credit. The unique index on phone_hash is the barrier that two accounts
// created with the same phone in the same instant cannot pass: the shared
// signal check reads the history before either account wrote it, so only the
// database can refuse the second insert. Like every signal table it holds the
// keyed hash, never the raw phone number.
type HwFreeCreditClaim struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	PhoneHash string `json:"phone_hash" gorm:"type:varchar(64);not null;uniqueIndex"`
	UserId    int    `json:"user_id" gorm:"not null;index"`
	CreatedAt int64  `json:"created_at" gorm:"not null;default:0"`
}

func (HwFreeCreditClaim) TableName() string {
	return "hw_free_credit_claims"
}

// HwGrantFreeTrialCredit grants the one-time free trial credit of a user. The
// amount comes from the caller, because converting the TRY amount needs the
// daily exchange rate; it may not exceed HwMaxFreeTrialQuota.
//
// The credit is refused when the user has no phone hash (reason unverified): a
// device token is supplied by the browser and renewable per account, so it does
// not verify a person. It is further refused when another account that shares a
// phone number or card fingerprint already received the free credit (reason
// shared_<kind>; the phone hash also reserves a unique claim row, so two
// accounts with the same phone cannot both pass in the same instant), or when
// the amount is above the cap (reason quota_too_high). A shared device token is
// no refusal: the browser chooses it, so a copy must not deny another account. Only the first call has
// an effect; the package expires after hwFreeCreditValiditySeconds. A repeated
// call on another day converts the TRY amount against another rate and
// therefore arrives with another quota; it is answered as already_granted
// without touching the existing package.
//
// A regular user in the upstream default group is moved to the free group. Paid,
// administrator and operator-assigned groups are never changed here.
func HwGrantFreeTrialCredit(userID int, freeQuota int, now int64) (granted bool, reason string, err error) {
	if userID <= 0 || freeQuota <= 0 || now <= 0 {
		return false, "", ErrHwInvalidCreditGrant
	}
	if freeQuota > HwMaxFreeTrialQuota() {
		return false, HwFreeTrialQuotaTooHigh, nil
	}
	phoneHashes, err := hwNewestSignalHashes(DB, userID, []string{HwSignalKindPhone})
	if err != nil {
		return false, "", err
	}
	if phoneHashes[HwSignalKindPhone] == "" {
		return false, HwFreeTrialUnverified, nil
	}
	sharedKind, err := hwFreeTrialSharedSignalKind(userID)
	if err != nil {
		return false, "", err
	}
	if sharedKind != "" {
		return false, HwFreeTrialSharedSignal + sharedKind, nil
	}
	phoneHash := phoneHashes[HwSignalKindPhone]
	// The claim row, the package and the balance credit are written in one
	// transaction: a phone hash that loses the race must not leave a claim
	// behind without a package, and a package failure must not reserve the
	// phone hash. The TRY amount is converted against the daily rate, so a
	// repeated call on another day arrives with another quota; the free package
	// that already exists is the truth, not the amount of today.
	err = DB.Transaction(func(tx *gorm.DB) error {
		claim := HwFreeCreditClaim{PhoneHash: phoneHash, UserId: userID, CreatedAt: now}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "phone_hash"}},
			DoNothing: true,
		}).Create(&claim)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var existing HwFreeCreditClaim
			if err := tx.Where("phone_hash = ?", phoneHash).First(&existing).Error; err != nil {
				return err
			}
			if existing.UserId != userID {
				reason = HwFreeTrialSharedSignal + HwSignalKindPhone
				return nil
			}
			// The same user claiming again, possibly with a phone it changed to
			// in between: the package check below decides the answer.
		}
		var freePackages int64
		if err := tx.Model(&HwCreditPackage{}).
			Where("user_id = ? AND source = ?", userID, HwSourceFree).
			Count(&freePackages).Error; err != nil {
			return err
		}
		if freePackages > 0 {
			return nil
		}
		var err error
		granted, err = GrantCreditPackage(tx, userID, freeQuota, HwSourceFree, "free:"+strconv.Itoa(userID), now)
		return err
	})
	if err != nil {
		return false, "", err
	}
	if reason != "" {
		return false, reason, nil
	}
	if granted {
		// GrantCreditPackage ran on the caller's transaction, so the cache sync
		// that it normally does itself belongs here, after the commit.
		HwSyncCachesAfterGrant(userID, freeQuota, HwSourceFree)
	}
	// Run the group move even when the package already existed: a previous call
	// that failed after its commit must not leave the user in the default group.
	result := DB.Model(&User{}).
		Where("id = ? AND role = ? AND "+commonGroupCol+" = ?", userID, common.RoleCommonUser, hwGroupDefault).
		Update("group", hwGroupFree)
	if result.Error != nil {
		return granted, "", result.Error
	}
	if result.RowsAffected > 0 {
		if err := RefreshUserGroupCache(userID); err != nil {
			common.SysError(common.LogText("failed to refresh user group cache after free trial credit for user %d: %v", userID, err))
		}
	}
	if !granted {
		return false, HwFreeTrialAlreadyGranted, nil
	}
	return true, "", nil
}

// hwFreeTrialSharedSignalKind reports a signal kind on which another account
// already received the free credit. It looks at the whole signal history, so
// changing a phone number or a card does not clear the match. The device token
// is deliberately absent: the browser chooses it, so anyone who copies a token
// could otherwise deny the free credit of the account that owns it.
func hwFreeTrialSharedSignalKind(userID int) (string, error) {
	for _, kind := range []string{HwSignalKindPhone, HwSignalKindCard} {
		var hashes []string
		if err := DB.Model(&HwUserSignal{}).
			Where("user_id = ? AND kind = ? AND hash <> ?", userID, kind, "").
			Pluck("hash", &hashes).Error; err != nil {
			return "", err
		}
		if len(hashes) == 0 {
			continue
		}
		var otherUserIDs []int
		if err := DB.Model(&HwUserSignal{}).
			Where("user_id <> ? AND kind = ? AND hash IN ?", userID, kind, hashes).
			Distinct().
			Pluck("user_id", &otherUserIDs).Error; err != nil {
			return "", err
		}
		if len(otherUserIDs) == 0 {
			continue
		}
		var count int64
		if err := DB.Model(&HwCreditPackage{}).
			Where("user_id IN ? AND source = ?", otherUserIDs, HwSourceFree).
			Count(&count).Error; err != nil {
			return "", err
		}
		if count > 0 {
			return kind, nil
		}
	}
	return "", nil
}
