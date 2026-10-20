package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwDisposableTestSetup neutralises the optional upstream email restrictions so
// the disposable check is the only refusal that can fire.
func hwDisposableTestSetup(t *testing.T) {
	t.Helper()
	previousDomain, previousAlias := common.EmailDomainRestrictionEnabled, common.EmailAliasRestrictionEnabled
	common.EmailDomainRestrictionEnabled = false
	common.EmailAliasRestrictionEnabled = false
	t.Cleanup(func() {
		common.EmailDomainRestrictionEnabled = previousDomain
		common.EmailAliasRestrictionEnabled = previousAlias
	})
}

func TestHwEmailDomainBlocked(t *testing.T) {
	t.Run("known disposable domains are blocked in every spelling", func(t *testing.T) {
		for _, email := range []string{
			"user@mailinator.com",
			"user@MAILINATOR.com",
			"user@Mailinator.Com",
			"some.one+tag@10minutemail.com",
			"dotted.local@yopmail.fr",
			"user@temp-mail.org",
			"user@guerrillamail.com",
		} {
			assert.True(t, HwEmailDomainBlocked(email), "%s must be blocked", email)
			require.ErrorIs(t, HwRejectDisposableEmail(email), ErrHwDisposableEmail, "%s must be refused", email)
		}
	})

	t.Run("subdomains of a blocked domain are blocked", func(t *testing.T) {
		for _, email := range []string{
			"user@mail.mailinator.com",
			"user@deep.sub.guerrillamail.org",
			"user@x.y.z.temp-mail.io",
		} {
			assert.True(t, HwEmailDomainBlocked(email), "%s must be blocked", email)
		}
	})

	t.Run("plus addressing and dots do not change the domain", func(t *testing.T) {
		assert.True(t, HwEmailDomainBlocked("a+b+c@sharklasers.com"))
		assert.True(t, HwEmailDomainBlocked("a.b.c@sharklasers.com"))
		assert.True(t, HwEmailDomainBlocked(" a+b@SHARKLASERS.com "))
	})

	t.Run("ordinary domains are allowed", func(t *testing.T) {
		for _, email := range []string{
			"user@gmail.com",
			"user@hotmail.com",
			"user@outlook.com",
			"user@hisarweb.com.tr",
			"user@sub.example.com",
			"user@proton.me",
			"user@yahoo.com",
		} {
			assert.False(t, HwEmailDomainBlocked(email), "%s must be allowed", email)
			assert.NoError(t, HwRejectDisposableEmail(email))
		}
	})

	t.Run("addresses without a domain are not blocked here", func(t *testing.T) {
		assert.False(t, HwEmailDomainBlocked("not-an-address"))
		assert.False(t, HwEmailDomainBlocked(""))
	})

	t.Run("the extra domains option extends the list", func(t *testing.T) {
		t.Setenv("HW_EXTRA_BLOCKED_DOMAINS", " Example-Org.COM , second.example ")
		HwReloadHwDisposableDomains()
		t.Cleanup(HwReloadHwDisposableDomains)

		assert.True(t, HwEmailDomainBlocked("user@example-org.com"), "an added domain blocks its subdomains too")
		assert.True(t, HwEmailDomainBlocked("user@second.example"))
		assert.False(t, HwEmailDomainBlocked("user@example-org.org"), "only the added domain and its subdomains are blocked")
	})

	t.Run("the embedded list has at least 200 domains", func(t *testing.T) {
		HwReloadHwDisposableDomains()
		assert.Greater(t, len(hwDisposableDomainSet), 200)
	})
}

func TestValidateAccountEmailRejectsDisposableDomains(t *testing.T) {
	hwDisposableTestSetup(t)

	t.Run("the verification path refuses a disposable address", func(t *testing.T) {
		_, err := ValidateAccountEmail("customer@mailinator.com")
		require.ErrorIs(t, err, ErrHwDisposableEmail)
	})

	t.Run("the verification path refuses a subdomain of a disposable address", func(t *testing.T) {
		_, err := ValidateAccountEmail("customer@mail.mailinator.com")
		require.ErrorIs(t, err, ErrHwDisposableEmail)
	})

	t.Run("an ordinary address still passes", func(t *testing.T) {
		email, err := ValidateAccountEmail("Customer@Example.com ")
		require.NoError(t, err)
		assert.Equal(t, "customer@example.com", email)
	})
}
