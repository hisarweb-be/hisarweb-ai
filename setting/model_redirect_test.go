package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoreModelRedirects(t *testing.T) {
	t.Helper()
	previous := ModelRedirects2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRedirectsByJSONString(previous))
	})
}

func TestModelRedirectsLookupAndRoundTrip(t *testing.T) {
	restoreModelRedirects(t)

	require.NoError(t, UpdateModelRedirectsByJSONString(`{"gpt-5.6-luna":"gpt-6-luna","gpt-5.6-sol":"gpt-6.1-sol"}`))

	target, ok := GetModelRedirect("gpt-5.6-luna")
	assert.True(t, ok)
	assert.Equal(t, "gpt-6-luna", target)

	_, ok = GetModelRedirect("gpt-6-luna")
	assert.False(t, ok, "a successor must not redirect again unless configured")

	assert.JSONEq(t, `{"gpt-5.6-luna":"gpt-6-luna","gpt-5.6-sol":"gpt-6.1-sol"}`, ModelRedirects2JSONString())
}

func TestModelRedirectsRejectBrokenTables(t *testing.T) {
	restoreModelRedirects(t)

	require.NoError(t, UpdateModelRedirectsByJSONString(`{"a":"b"}`))

	for name, jsonTable := range map[string]string{
		"empty target":     `{"a":""}`,
		"self mapping":     `{"a":"a"}`,
		"direct cycle":     `{"a":"b","b":"a"}`,
		"longer cycle":     `{"a":"b","b":"c","c":"a"}`,
		"invalid json":     `{"a":`,
		"non string value": `{"a":42}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, UpdateModelRedirectsByJSONString(jsonTable))
			// A rejected table must not corrupt the running one.
			target, ok := GetModelRedirect("a")
			require.True(t, ok)
			assert.Equal(t, "b", target)
		})
	}
}
