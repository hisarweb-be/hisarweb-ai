package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G8: token and e-mail are in the fragment, never in the query string.
func TestHwPasswordResetLinkUsesFragment(t *testing.T) {
	link := HwPasswordResetLink("https://ai.example.com/", "a+b@example.com", "tok123")
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "/user/reset", parsed.Path)
	assert.Empty(t, parsed.RawQuery, "nothing sensitive may reach the server or its logs")

	values, err := url.ParseQuery(parsed.EscapedFragment())
	require.NoError(t, err)
	assert.Equal(t, "a+b@example.com", values.Get("email"))
	assert.Equal(t, "tok123", values.Get("token"))
}
