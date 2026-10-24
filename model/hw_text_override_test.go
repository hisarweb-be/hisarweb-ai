package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHwValidateTextOverride(t *testing.T) {
	cases := []struct {
		name, lang, key, value string
		ok                     bool
	}{
		{"plain text", "tr", "Save", "Kaydet", true},
		{"placeholders kept in another order", "tr", "{{a}} of {{b}}", "{{b}} / {{a}}", true},
		{"placeholder dropped", "tr", "Hello {{name}}", "Merhaba", false},
		{"placeholder added", "tr", "Hello", "Merhaba {{name}}", false},
		{"placeholder renamed", "tr", "Hello {{name}}", "Merhaba {{ad}}", false},
		{"markup refused", "tr", "Save", "<b>Kaydet</b>", false},
		{"markup allowed when the key has it", "tr", "Save <b>now</b>", "<b>Şimdi</b> kaydet", true},
		{"empty value", "tr", "Save", "   ", false},
		{"empty key", "tr", " ", "Kaydet", false},
		{"unknown language", "xx", "Save", "Kaydet", false},
		{"value too long", "tr", "Save", strings.Repeat("a", HwTextOverrideMaxValueLen+1), false},
		{"value at the limit", "tr", "Save", strings.Repeat("a", HwTextOverrideMaxValueLen), true},
		{"key too long", "tr", strings.Repeat("k", HwTextOverrideMaxKeyLen+1), "Kaydet", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := HwValidateTextOverride(tc.lang, tc.key, tc.value)
			if tc.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestHwTextOverrideStoreReplaceListDelete(t *testing.T) {
	hwCreditTestSetup(t)
	db := DB
	require.NoError(t, db.AutoMigrate(&HwTextOverride{}))
	require.NoError(t, db.Exec("DELETE FROM hw_text_overrides").Error)
	t.Cleanup(func() { db.Exec("DELETE FROM hw_text_overrides") })

	require.NoError(t, HwSetTextOverride(db, "tr", "Save", "Sakla", 7, 100))
	require.NoError(t, HwSetTextOverride(db, "tr", "Save", "Kaydet", 8, 200), "a second apply replaces the first")
	require.NoError(t, HwSetTextOverride(db, "en", "Save", "Store", 8, 300))
	assert.Error(t, HwSetTextOverride(db, "tr", "Hi {{n}}", "Selam", 8, 400), "an invalid text is not stored")

	tr, err := HwTextOverridesFor(db, "tr")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Save": "Kaydet"}, tr)

	rows, err := HwListTextOverrides(db)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "en", rows[0].Lang, "newest first")

	removed, err := HwDeleteTextOverride(db, "tr", "Save")
	require.NoError(t, err)
	assert.True(t, removed)
	removed, err = HwDeleteTextOverride(db, "tr", "Save")
	require.NoError(t, err)
	assert.False(t, removed, "deleting an unknown override reports nothing removed")
	tr, err = HwTextOverridesFor(db, "tr")
	require.NoError(t, err)
	assert.Empty(t, tr)
}
