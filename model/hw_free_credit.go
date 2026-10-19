package model

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

// HwGrantFreeTrialCredit grants the one-time free trial credit of a user. The
// amount comes from the caller, because converting the TRY amount needs the
// daily exchange rate. Only the first call has an effect; the package expires
// after HwFreeCreditValiditySeconds.
//
// A regular user in the upstream default group is moved to the free group. Paid,
// administrator and operator-assigned groups are never changed here.
func HwGrantFreeTrialCredit(userID int, freeQuota int, now int64) (bool, error) {
	granted, err := GrantCreditPackage(nil, userID, freeQuota, HwSourceFree, "free:"+strconv.Itoa(userID), now)
	if err != nil {
		return false, err
	}
	// Run the group move even when the package already existed: a previous call
	// that failed after its commit must not leave the user in the default group.
	result := DB.Model(&User{}).
		Where("id = ? AND role = ? AND "+commonGroupCol+" = ?", userID, common.RoleCommonUser, hwGroupDefault).
		Update("group", hwGroupFree)
	if result.Error != nil {
		return granted, result.Error
	}
	if result.RowsAffected > 0 {
		if err := RefreshUserGroupCache(userID); err != nil {
			common.SysError(common.LogText("failed to refresh user group cache after free trial credit for user %d: %v", userID, err))
		}
	}
	return granted, nil
}
