package model

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Sweep thresholds come from the environment so operations can tune them
// without a release. Defaults sit on the safe side: two accounts on one phone,
// device or card is fraud-shaped, thirty signups in one hour or five accounts
// from one address in one day is a wave.
const (
	hwSweepSharedUsersEnv = "HW_SWEEP_SHARED_USERS"
	hwSweepHourWaveEnv    = "HW_SWEEP_HOUR_WAVE"
	hwSweepIpDayWaveEnv   = "HW_SWEEP_IP_DAY_WAVE"

	hwSweepDefaultSharedUsers = 2
	hwSweepDefaultHourWave    = 30
	hwSweepDefaultIpDayWave   = 5

	hwSweepWindow   = 24 * time.Hour
	hwSweepInterval = 24 * time.Hour
)

// HwAbuseSweepReport carries what one sweep pass did. Every field is a count;
// no signal value, address or account name ever reaches it or the log.
type HwAbuseSweepReport struct {
	SharedPhone  int
	SharedDevice int
	SharedCard   int
	SignupWave   int
	FrozenTotal  int
}

func hwSweepThreshold(env string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(env)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 2 {
			return parsed
		}
	}
	return fallback
}

// HwDailyAbuseSweep is the nightly panic net behind the inline checks. It
// freezes common-user accounts for review: (a) accounts that share a phone,
// device or card signal with another account, (b) signup waves, meaning an
// unusually full registration hour or an address that produced an unusual
// number of accounts in one day. Freezing never touches balances: paid credit
// and the remaining balance stay exactly as they are, and the block only
// applies to API usage that draws on the free trial credit. An administrator
// thaws accounts after review. Log lines carry counts only.
func HwDailyAbuseSweep(now int64) (*HwAbuseSweepReport, error) {
	report := &HwAbuseSweepReport{}
	sharedUsers := hwSweepThreshold(hwSweepSharedUsersEnv, hwSweepDefaultSharedUsers)

	// (a) Shared phone, device and card signals. The IP signal stays out: one
	// address legitimately serves a whole household or office, and the per-day
	// signup counter already caps what an address can create.
	for _, kind := range []struct {
		signalKind string
		reason     string
		count      *int
	}{
		{HwSignalKindPhone, HwFreezeReasonSharedPhone, &report.SharedPhone},
		{HwSignalKindDevice, HwFreezeReasonSharedDevice, &report.SharedDevice},
		{HwSignalKindCard, HwFreezeReasonSharedCard, &report.SharedCard},
	} {
		ids, err := hwUserIDsOnSharedSignal(kind.signalKind, sharedUsers)
		if err != nil {
			return report, err
		}
		frozen, err := FreezeHwAccounts(ids, kind.reason, now)
		if err != nil {
			return report, err
		}
		*kind.count = frozen
		report.FrozenTotal += frozen
	}

	// (b) Signup waves. Registration hour buckets over the last 24 hours: an
	// hour at or above the threshold is a wave, and the accounts it created go
	// to review. Users created before the window are never touched.
	wave := hwSweepThreshold(hwSweepHourWaveEnv, hwSweepDefaultHourWave)
	waveUserIDs, err := hwSignupWaveUserIDs(now, wave)
	if err != nil {
		return report, err
	}
	waveFrozen, err := FreezeHwAccounts(waveUserIDs, HwFreezeReasonSignupWave, now)
	if err != nil {
		return report, err
	}
	report.SignupWave = waveFrozen
	report.FrozenTotal += waveFrozen

	// Per-address wave: five or more accounts whose signup ip signal was first
	// seen inside the window on the same address hash.
	ipWave := hwSweepThreshold(hwSweepIpDayWaveEnv, hwSweepDefaultIpDayWave)
	ipUserIDs, err := hwIpWaveUserIDs(now, ipWave)
	if err != nil {
		return report, err
	}
	ipFrozen, err := FreezeHwAccounts(ipUserIDs, HwFreezeReasonSignupWave, now)
	if err != nil {
		return report, err
	}
	report.SignupWave += ipFrozen
	report.FrozenTotal += ipFrozen

	if err := HwDeleteOldSignupIpRows(now, 2); err != nil {
		return report, err
	}
	if report.FrozenTotal > 0 {
		common.SysLog(common.LogText("hw: abuse sweep froze %d accounts (shared phone %d, shared device %d, shared card %d, signup waves %d)",
			report.FrozenTotal, report.SharedPhone, report.SharedDevice, report.SharedCard, report.SignupWave))
	}
	return report, nil
}

// hwUserIDsOnSharedSignal returns the users that share a signal value of the
// kind with at least the given number of distinct accounts.
func hwUserIDsOnSharedSignal(kind string, sharedUsers int) ([]int, error) {
	var sharedHashes []string
	if err := DB.Model(&HwUserSignal{}).
		Select("hash").
		Where("kind = ? AND hash <> ''", kind).
		Group("hash").
		Having("COUNT(DISTINCT user_id) >= ?", sharedUsers).
		Pluck("hash", &sharedHashes).Error; err != nil {
		return nil, err
	}
	if len(sharedHashes) == 0 {
		return nil, nil
	}
	var userIDs []int
	if err := DB.Model(&HwUserSignal{}).
		Where("kind = ? AND hash IN ?", kind, sharedHashes).
		Distinct().
		Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	return userIDs, nil
}

// hwSignupWaveUserIDs finds users created in a registration hour that reached
// the wave threshold inside the window. created_at is a unix timestamp, so the
// bucket is plain integer division; all three supported databases run it on
// the integer column.
func hwSignupWaveUserIDs(now int64, wave int) ([]int, error) {
	var buckets []int64
	if err := DB.Model(&User{}).
		Select("created_at / 3600 AS bucket").
		Where("created_at >= ?", now-int64(hwSweepWindow/time.Second)).
		Group("bucket").
		Having("COUNT(*) >= ?", wave).
		Pluck("bucket", &buckets).Error; err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return nil, nil
	}
	// Pluck per bucket: a range between the first and last wave bucket would
	// sweep in the quiet hours between two separate waves.
	var userIDs []int
	for _, bucket := range buckets {
		var inBucket []int
		if err := DB.Model(&User{}).
			Where("created_at >= ? AND created_at < ?", bucket*3600, (bucket+1)*3600).
			Pluck("id", &inBucket).Error; err != nil {
			return nil, err
		}
		userIDs = append(userIDs, inBucket...)
	}
	return userIDs, nil
}

// hwIpWaveUserIDs finds users whose signup address hash carried at least the
// given number of distinct accounts whose ip signal first appeared inside the
// window.
func hwIpWaveUserIDs(now int64, ipWave int) ([]int, error) {
	windowStart := now - int64(hwSweepWindow/time.Second)
	var sharedHashes []string
	if err := DB.Model(&HwUserSignal{}).
		Select("hash").
		Where("kind = ? AND hash <> '' AND first_seen >= ?", HwSignalKindIp, windowStart).
		Group("hash").
		Having("COUNT(DISTINCT user_id) >= ?", ipWave).
		Pluck("hash", &sharedHashes).Error; err != nil {
		return nil, err
	}
	if len(sharedHashes) == 0 {
		return nil, nil
	}
	var userIDs []int
	if err := DB.Model(&HwUserSignal{}).
		Where("kind = ? AND hash IN ? AND first_seen >= ?", HwSignalKindIp, sharedHashes, windowStart).
		Distinct().
		Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	return userIDs, nil
}

var hwAbuseSweepOnce sync.Once

// StartHwDailyAbuseSweepJob starts the daily abuse sweep. It runs once per
// process and only on the master node; the sweep is idempotent, so an extra
// pass after a restart is harmless.
func StartHwDailyAbuseSweepJob() {
	hwAbuseSweepOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog(common.LogText("hw: daily abuse sweep job started: interval=%s", hwSweepInterval))
			runHwAbuseSweepSafely()
			ticker := time.NewTicker(hwSweepInterval)
			defer ticker.Stop()
			for range ticker.C {
				runHwAbuseSweepSafely()
			}
		}()
	})
}

func runHwAbuseSweepSafely() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("hw: daily abuse sweep panicked: %v", r))
		}
	}()
	if _, err := HwDailyAbuseSweep(common.GetTimestamp()); err != nil {
		common.SysError(common.LogText("hw: daily abuse sweep failed: %v", err))
	}
}
