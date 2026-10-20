package service

import (
	"embed"
	"errors"
	"os"
	"strings"
)

// The embedded list of known disposable email domains. go:embed keeps the
// list in the binary, so the check works without network access and without
// an operator installing anything.
//
//go:embed hw_disposable_domains.txt
var hwDisposableDomainsFS embed.FS

// hwExtraBlockedDomainsEnv names the comma-separated environment variable with
// operator-added domains. Only its name appears in logs and errors.
const hwExtraBlockedDomainsEnv = "HW_EXTRA_BLOCKED_DOMAINS"

// ErrHwDisposableEmail refuses a signup address on a disposable email domain.
// The error text never repeats the address or the domain.
var ErrHwDisposableEmail = errors.New("This email provider is not allowed. Please use a permanent email address.")

// hwDisposableDomainSet is the parsed embedded list plus the extra domains of
// the environment. It is built once per process.
var hwDisposableDomainSet = loadHwDisposableDomains()

// loadHwDisposableDomains parses the embedded list and the extra domains into
// one set. Lines that are not a plain lowercase domain are ignored, so a
// hand-edited file cannot break startup.
func loadHwDisposableDomains() map[string]struct{} {
	set := make(map[string]struct{})
	raw, err := hwDisposableDomainsFS.ReadFile("hw_disposable_domains.txt")
	if err != nil {
		return set
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[line] = struct{}{}
	}
	for domain := range strings.SplitSeq(strings.ToLower(os.Getenv(hwExtraBlockedDomainsEnv)), ",") {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			set[domain] = struct{}{}
		}
	}
	return set
}

// HwReloadHwDisposableDomains rebuilds the blocked-domain set from the embedded
// list and the current environment. Tests use it after changing the extra
// domains; production reads the environment once at startup.
func HwReloadHwDisposableDomains() {
	hwDisposableDomainSet = loadHwDisposableDomains()
}

// HwEmailDomainBlocked reports whether the domain of an email address, or any
// parent domain of it, is on the disposable list. The address is normalised
// first: case never matters, and plus-addressing or dots in the local part do
// not change the domain, so they cannot dodge the check.
func HwEmailDomainBlocked(email string) bool {
	_, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || domain == "" {
		return false
	}
	// Walk the domain from the exact name up to its registrable suffixes:
	// "a.b.mailinator.com" checks itself, then "b.mailinator.com", then
	// "mailinator.com", then "com".
	labels := strings.Split(domain, ".")
	for i := range len(labels) {
		if _, blocked := hwDisposableDomainSet[strings.Join(labels[i:], ".")]; blocked {
			return true
		}
	}
	return false
}

// HwRejectDisposableEmail refuses an email address whose domain is a known
// disposable provider. Registration and every email-verification path call
// this before accepting or mailing to an address.
func HwRejectDisposableEmail(email string) error {
	if HwEmailDomainBlocked(email) {
		return ErrHwDisposableEmail
	}
	return nil
}
