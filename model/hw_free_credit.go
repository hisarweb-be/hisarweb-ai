package model

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
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

// HwGrantFreeTrialCredit grants the one-time free trial credit of a user. The
// amount comes from the caller, because converting the TRY amount needs the
// daily exchange rate; it may not exceed HwMaxFreeTrialQuota.
//
// The credit is refused when the user has no phone hash (reason unverified): a
// device token is supplied by the browser and renewable per account, so it does
// not verify a person. It is further refused when another account that shares a
// device, phone number or card fingerprint already received the free credit
// (reason shared_<kind>), or when the amount is above the cap (reason
// quota_too_high). Only the first call has an effect; the package expires after
// hwFreeCreditValiditySeconds. A repeated call on another day converts the TRY
// amount against another rate and therefore arrives with another quota; it is
// answered as already_granted without touching the existing package.
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
	// The TRY amount is converted against the daily rate, so a repeated call on
	// another day arrives with another quota. The free package that already
	// exists is the truth, not the amount of today: look for it first, or the
	// idempotence check of the ledger would report a conflict on the reference.
	var freePackages int64
	if err := DB.Model(&HwCreditPackage{}).
		Where("user_id = ? AND source = ?", userID, HwSourceFree).
		Count(&freePackages).Error; err != nil {
		return false, "", err
	}
	if freePackages == 0 {
		granted, err = GrantCreditPackage(nil, userID, freeQuota, HwSourceFree, "free:"+strconv.Itoa(userID), now)
		if err != nil {
			return false, "", err
		}
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
// changing a device or an IP address does not clear the match.
func hwFreeTrialSharedSignalKind(userID int) (string, error) {
	for _, kind := range []string{HwSignalKindDevice, HwSignalKindPhone, HwSignalKindCard} {
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
