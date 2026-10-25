package model

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// hwSignalSaltEnv names the server-side secret used to hash phone numbers, IP
// addresses and card fingerprints before they are stored. There is deliberately
// no default salt in code: without a secret nothing can be hashed.
const hwSignalSaltEnv = "HW_SIGNAL_SALT"

// hwSignalSaltMinBytes is the shortest HW_SIGNAL_SALT that may key the signal
// hashes. A secret below it is refused instead of silently weakening every
// keyed hash.
const hwSignalSaltMinBytes = 32

// ErrHwSignalSaltMissing is returned when HW_SIGNAL_SALT is not configured.
// There is no fallback: the hash of a phone number, IP address or card
// fingerprint must be keyed with a secret that is dedicated to this purpose,
// because the server crypto secret can be rotated for other reasons and would
// silently change every stored hash. Callers must treat the error as "do not
// store the signal".
var ErrHwSignalSaltMissing = errors.New("HW_SIGNAL_SALT is not configured")

// ErrHwSignalSaltTooShort is returned when HW_SIGNAL_SALT holds fewer than
// hwSignalSaltMinBytes bytes. A short salt would weaken every keyed hash, so it
// is refused with the same "do not store the signal" meaning as a missing one.
var ErrHwSignalSaltTooShort = errors.New("HW_SIGNAL_SALT is shorter than 32 bytes")

// ErrHwInvalidUserSignal rejects a signal write for an unknown user.
var ErrHwInvalidUserSignal = errors.New("invalid user signal")

// ErrHwInvalidPhoneNumber rejects a phone number that is not in one of the
// accepted spellings or does not belong to an accepted country code.
var ErrHwInvalidPhoneNumber = errors.New("invalid phone number")

// hwPhoneCountryCodesEnv names the comma-separated country codes a phone number
// may carry. The first code completes a national spelling.
const hwPhoneCountryCodesEnv = "HW_PHONE_COUNTRY_CODES"

// hwDefaultPhoneCountryCode is the code a national spelling (without a country
// code) is completed with when HW_PHONE_COUNTRY_CODES is unset.
const hwDefaultPhoneCountryCode = "90"

// hwDefaultPhoneCountryCodes apply when HW_PHONE_COUNTRY_CODES is unset, empty
// or holds no usable code: Turkey first, then the European countries where most
// of our Turkish-speaking customers live (Germany, the Netherlands, Belgium,
// France, Austria, Switzerland, the United Kingdom, Sweden, Denmark). A customer
// there shares a local SIM, and refusing it left them stuck on the phone step.
var hwDefaultPhoneCountryCodes = []string{hwDefaultPhoneCountryCode, "49", "31", "32", "33", "43", "41", "44", "46", "45"}

// Signal kinds recorded in hw_user_signals. Every kind is hashed before it is
// stored; the table never holds a raw address, number or fingerprint.
const (
	HwSignalKindDevice = "device"
	HwSignalKindIp     = "ip"
	HwSignalKindPhone  = "phone"
	HwSignalKindCard   = "card"
	// HwSignalKindTelegram records the Telegram identity of an account. It is
	// deliberately not the phone kind: the login widget gives no number, and one
	// person can retire a Telegram account and create another with the same SIM.
	// Only a number shared through the bot's contact button is a phone signal.
	HwSignalKindTelegram = "telegram"
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

// hwDropLegacyUserSignalsTable removes the pre-ledger shape of hw_user_signals,
// which stored one column per signal kind (device_token, ...) instead of the
// kind/hash history rows. AutoMigrate only adds columns, so without this drop a
// database that still has the old shape would keep it forever. Dropping is safe:
// the table was introduced in this pull request and holds no production data
// yet, and migrateDB rebuilds it in the current shape right afterwards.
func hwDropLegacyUserSignalsTable(db *gorm.DB) error {
	if !db.Migrator().HasTable(&HwUserSignal{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&HwUserSignal{}, "device_token") {
		return nil
	}
	return db.Migrator().DropTable(&HwUserSignal{})
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
// instead of falling back to the server crypto secret, and a salt shorter than
// hwSignalSaltMinBytes with ErrHwSignalSaltTooShort.
func HwSignalSalt() (string, error) {
	salt := strings.TrimSpace(os.Getenv(hwSignalSaltEnv))
	if salt == "" {
		return "", ErrHwSignalSaltMissing
	}
	if len(salt) < hwSignalSaltMinBytes {
		return "", ErrHwSignalSaltTooShort
	}
	return salt, nil
}

// HwHashSignalValue returns the keyed hash stored for a raw IP address, phone
// number or card fingerprint. Callers must not store the raw value instead.
// Surrounding whitespace is dropped, but the case of the value is kept: only
// hwHashRawSignal decides per kind whether a value is case-insensitive, so a
// device token or a card fingerprint keeps the exact spelling the outside world
// gave it.
func HwHashSignalValue(value string) (string, error) {
	value = strings.TrimSpace(value)
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

// hwPhoneCountryCodes reads the country codes a number may carry from
// HW_PHONE_COUNTRY_CODES (comma separated, a leading plus is dropped). Without
// a usable setting hwDefaultPhoneCountryCodes apply.
func hwPhoneCountryCodes() []string {
	codes := make([]string, 0, 4)
	for part := range strings.SplitSeq(os.Getenv(hwPhoneCountryCodesEnv), ",") {
		code := strings.TrimPrefix(strings.TrimSpace(part), "+")
		if code == "" || len(code) > 3 {
			continue
		}
		digits := true
		for _, r := range code {
			if r < '0' || r > '9' {
				digits = false
				break
			}
		}
		if digits {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return hwDefaultPhoneCountryCodes
	}
	return codes
}

// hwValidPhoneNationalLength checks the part of a number after its country
// code. Turkish numbers are always ten digits (three-digit area code plus seven
// digits); another accepted country code is bound by the fifteen-digit total of
// E.164 instead, because national lengths differ per country.
func hwValidPhoneNationalLength(code string, national int) bool {
	if code == hwDefaultPhoneCountryCode {
		return national == 10
	}
	return national >= 6 && len(code)+national <= 15
}

// hwNormalizePhone reduces a phone number to E.164 (+90XXXXXXXXXX by default).
// Accepted spellings are "+90 555 123 45 67", "905551234567", "0555 123 45 67"
// and a bare "555 123 45 67"; spaces, dashes, dots, parentheses and a leading
// plus are separators. Another accepted country needs the plus ("+49 151 ...").
// A number that carries a country code outside the accepted codes is refused, and so is anything that does not leave a
// national part of the right length: two spellings of one number must produce
// one hash, and an unrecognisable or foreign number must not be stored at all.
func hwNormalizePhone(raw string) (string, error) {
	var digits strings.Builder
	international := false
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && i == 0:
			international = true
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
			// Separator only, carries no value.
		default:
			return "", ErrHwInvalidPhoneNumber
		}
	}
	number := digits.String()
	codes := hwPhoneCountryCodes()
	// Without the plus only the first code is read as a country code: a bare
	// national spelling such as Ankara's "312 ..." must not turn into +31.
	candidates := codes
	if !international {
		candidates = codes[:1]
	}
	for _, code := range candidates {
		if national, ok := strings.CutPrefix(number, code); ok && hwValidPhoneNationalLength(code, len(national)) {
			return "+" + code + national, nil
		}
	}
	if international {
		// The plus promises a country code; one we do not accept is refused
		// instead of being read as a national number.
		return "", ErrHwInvalidPhoneNumber
	}
	// A national spelling: one trunk zero is dropped and the default country
	// code is prepended.
	code := codes[0]
	national := strings.TrimPrefix(number, "0")
	if !hwValidPhoneNationalLength(code, len(national)) {
		return "", ErrHwInvalidPhoneNumber
	}
	return "+" + code + national, nil
}

// hwIpSignalValue reduces the address that the ip kind hashes. IPv4 keeps its
// dotted form after parsing. IPv6 is reduced to its /64 prefix: one customer
// normally controls a whole /64, so hashing the full address would let it walk
// past the per-address signup limit and hide from the daily sweep by rotating
// the lower 64 bits. The prefix is written in canonical network form, so every
// notation of one address maps to one value. A value that does not parse as an
// address is lowercased and hashed as given, as before.
func hwIpSignalValue(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	ip := net.ParseIP(value)
	if ip == nil {
		return value
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	mask := net.CIDRMask(64, 128)
	return (&net.IPNet{IP: ip.Mask(mask), Mask: mask}).String()
}

// hwHashRawSignal hashes one raw signal value of a kind, normalising it first
// where a single external form exists. Only an IP address is lowercased and
// reduced to its /64 prefix: its notation is case-insensitive by convention. A
// device token and a card fingerprint are opaque case-sensitive values, and a
// phone number is reduced to E.164 by hwNormalizePhone, which no case rule may
// disturb.
func hwHashRawSignal(kind, value string) (string, error) {
	if kind == HwSignalKindIp {
		value = hwIpSignalValue(value)
	}
	if kind == HwSignalKindPhone {
		phone, err := hwNormalizePhone(value)
		if err != nil {
			return "", err
		}
		value = phone
	}
	return HwHashSignalValue(value)
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
		hash, err := hwHashRawSignal(signal.kind, signal.value)
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
	// A second writer for a value the user already produced hits the unique
	// (user_id, kind, hash) index. Let the database resolve it: the first row
	// keeps its first_seen and only its last_seen moves. Selecting the row first
	// and inserting afterwards would still lose the race between two writers.
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "kind"}, {Name: "hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_seen"}),
	}).Create(&rows).Error
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
