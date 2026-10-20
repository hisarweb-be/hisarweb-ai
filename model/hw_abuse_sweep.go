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
	hwSweepDeviceUsersEnv = "HW_SWEEP_DEVICE_USERS"
	hwSweepHourWaveEnv    = "HW_SWEEP_HOUR_WAVE"
	hwSweepIpDayWaveEnv   = "HW_SWEEP_IP_DAY_WAVE"

	hwSweepDefaultSharedUsers = 2
	hwSweepDefaultDeviceUsers = 5
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
// freezes common-user accounts for review: (a) accounts that share a phone or
// card signal with another account, or a device token together with a second
// shared signal or across an unusual number of accounts, (b) signup waves,
// meaning an account of an unusually full registration hour that shares a
// second signal with another account of that hour or has no confirmed email
// address, or an address that produced an unusual number of accounts in one
// day. Freezing never touches balances: paid credit
// and the remaining balance stay exactly as they are, and the block only
// applies to API usage that draws on the free trial credit. An administrator
// thaws accounts after review. Log lines carry counts only.
func HwDailyAbuseSweep(now int64) (*HwAbuseSweepReport, error) {
	report := &HwAbuseSweepReport{}
	sharedUsers := hwSweepThreshold(hwSweepSharedUsersEnv, hwSweepDefaultSharedUsers)

	// (a) Shared phone and card signals. The IP signal stays out: one address
	// legitimately serves a whole household or office, and the per-day signup
	// counter already caps what an address can create. The device token has its
	// own rule below, because a browser-chosen token costs an attacker nothing.
	for _, kind := range []struct {
		signalKind string
		reason     string
		count      *int
	}{
		{HwSignalKindPhone, HwFreezeReasonSharedPhone, &report.SharedPhone},
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

	// A device token is renewable per account and a copied token says nothing
	// about the person holding it, so a shared device alone never freezes. An
	// account counts when it also shares a phone number, address hash or card
	// fingerprint with another account of the same device group, or when one
	// token turned up on at least HW_SWEEP_DEVICE_USERS accounts.
	deviceUsers := hwSweepThreshold(hwSweepDeviceUsersEnv, hwSweepDefaultDeviceUsers)
	deviceIDs, err := hwUserIDsOnSharedDevice(deviceUsers)
	if err != nil {
		return report, err
	}
	deviceFrozen, err := FreezeHwAccounts(deviceIDs, HwFreezeReasonSharedDevice, now)
	if err != nil {
		return report, err
	}
	report.SharedDevice = deviceFrozen
	report.FrozenTotal += deviceFrozen

	// (b) Signup waves. Registration hour buckets over the last 24 hours: an
	// hour at or above the threshold is a wave. The hour alone never freezes an
	// account, because the attacker who fills an hour also fills it with real
	// customers; only the accounts of the wave that share a second signal with
	// another account of that hour, or that have no confirmed email address,
	// go to review. Users created before the window are never touched.
	wave := hwSweepThreshold(hwSweepHourWaveEnv, hwSweepDefaultHourWave)
	waveUserIDs, err := hwSignupWaveUserIDs(now, wave, deviceUsers)
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

// hwUserIDsOnSharedDevice returns the accounts that a shared device token marks
// for review. The token is chosen by the browser and a copy costs nothing, so
// the token alone never freezes an account: an account counts only when it also
// shares a phone number, address hash or card fingerprint with another account
// of the same device group, or when one token appears on at least bulkUsers
// accounts — the shape of one person cycling through signups rather than one
// copied token.
func hwUserIDsOnSharedDevice(bulkUsers int) ([]int, error) {
	var hashes []string
	if err := DB.Model(&HwUserSignal{}).
		Select("hash").
		Where("kind = ? AND hash <> ?", HwSignalKindDevice, "").
		Group("hash").
		Having("COUNT(DISTINCT user_id) >= ?", 2).
		Pluck("hash", &hashes).Error; err != nil {
		return nil, err
	}
	var userIDs []int
	for _, hash := range hashes {
		var group []int
		if err := DB.Model(&HwUserSignal{}).
			Where("kind = ? AND hash = ?", HwSignalKindDevice, hash).
			Distinct().
			Pluck("user_id", &group).Error; err != nil {
			return nil, err
		}
		if len(group) < 2 {
			continue
		}
		if len(group) >= bulkUsers {
			userIDs = append(userIDs, group...)
			continue
		}
		second, err := hwUsersSharingSecondSignal(group)
		if err != nil {
			return nil, err
		}
		userIDs = append(userIDs, second...)
	}
	return userIDs, nil
}

// hwUsersSharingSecondSignal returns the users of one device group that also
// share a phone number, address hash or card fingerprint with another user of
// that same group. A second signal held by only one account of the group proves
// nothing about the group.
func hwUsersSharingSecondSignal(group []int) ([]int, error) {
	var signals []HwUserSignal
	if err := DB.Where("user_id IN ? AND kind IN ? AND hash <> ?", group,
		[]string{HwSignalKindPhone, HwSignalKindIp, HwSignalKindCard}, "").
		Find(&signals).Error; err != nil {
		return nil, err
	}
	usersBySignal := make(map[string]map[int]struct{}, len(signals))
	for _, signal := range signals {
		key := signal.Kind + "\x00" + signal.Hash
		if usersBySignal[key] == nil {
			usersBySignal[key] = make(map[int]struct{}, 2)
		}
		usersBySignal[key][signal.UserId] = struct{}{}
	}
	qualifying := make(map[int]struct{}, len(group))
	for _, users := range usersBySignal {
		if len(users) < 2 {
			continue
		}
		for userID := range users {
			qualifying[userID] = struct{}{}
		}
	}
	shared := make([]int, 0, len(qualifying))
	for _, userID := range group {
		if _, ok := qualifying[userID]; ok {
			shared = append(shared, userID)
		}
	}
	return shared, nil
}

// hwSignupWaveUserIDs finds the users created in a registration hour that
// reached the wave threshold inside the window and that the wave actually
// incriminates. created_at is a unix timestamp, so the bucket is plain integer
// division; all three supported databases run it on the integer column.
//
// A full hour is no proof against one account: the attacker who fills it also
// fills it with real customers, so only the accounts that carry a second
// signal of their own go to review (see hwWaveAccountsToReview).
func hwSignupWaveUserIDs(now int64, wave int, bulkUsers int) ([]int, error) {
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
		var inBucket []User
		if err := DB.Select("id", "email").
			Where("created_at >= ? AND created_at < ?", bucket*3600, (bucket+1)*3600).
			Find(&inBucket).Error; err != nil {
			return nil, err
		}
		toReview, err := hwWaveAccountsToReview(inBucket, bulkUsers)
		if err != nil {
			return nil, err
		}
		userIDs = append(userIDs, toReview...)
	}
	return userIDs, nil
}

// hwWaveAccountsToReview returns the accounts of one wave hour that carry a
// second signal of their own: an address hash or an email pattern shared with
// another account of the same hour, a device token on at least bulkUsers
// accounts of the hour, or no confirmed email address at all. The device token
// keeps its bulk threshold from the shared-device rule: a copied token costs
// an attacker nothing, and a token alone must never sweep an account into
// review.
func hwWaveAccountsToReview(users []User, bulkUsers int) ([]int, error) {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	var signals []HwUserSignal
	if len(ids) > 0 {
		if err := DB.Where("user_id IN ? AND kind IN ? AND hash <> ?", ids,
			[]string{HwSignalKindIp, HwSignalKindDevice}, "").Find(&signals).Error; err != nil {
			return nil, err
		}
	}
	bySignal := make(map[string]map[int]struct{}, len(signals))
	for _, signal := range signals {
		key := signal.Kind + "\x00" + signal.Hash
		if bySignal[key] == nil {
			bySignal[key] = make(map[int]struct{}, 2)
		}
		bySignal[key][signal.UserId] = struct{}{}
	}
	toReview := make(map[int]struct{}, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.Email) == "" {
			toReview[user.Id] = struct{}{}
		}
	}
	for key, group := range bySignal {
		if len(group) < 2 {
			continue
		}
		if strings.HasPrefix(key, HwSignalKindDevice+"\x00") && len(group) < bulkUsers {
			continue
		}
		for userID := range group {
			toReview[userID] = struct{}{}
		}
	}
	byPattern := make(map[string][]int, len(users))
	for _, user := range users {
		if pattern := hwEmailPattern(user.Email); pattern != "" {
			byPattern[pattern] = append(byPattern[pattern], user.Id)
		}
	}
	for _, group := range byPattern {
		if len(group) < 2 {
			continue
		}
		for _, userID := range group {
			toReview[userID] = struct{}{}
		}
	}
	reviewed := make([]int, 0, len(toReview))
	for _, user := range users {
		if _, ok := toReview[user.Id]; ok {
			reviewed = append(reviewed, user.Id)
		}
	}
	return reviewed, nil
}

// hwEmailPattern reduces an address to the shape a repeat signup keeps: lower
// case, the local part without the +tag and without dots (the two spellings
// every large provider delivers to the same mailbox) and the domain unchanged.
// It returns "" for an address without a usable local part, so accounts
// without an email never group on the empty pattern.
func hwEmailPattern(email string) string {
	local, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || domain == "" {
		return ""
	}
	local, _, _ = strings.Cut(local, "+")
	local = strings.ReplaceAll(local, ".", "")
	if local == "" {
		return ""
	}
	return local + "@" + domain
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
