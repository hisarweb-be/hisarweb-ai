package model

// HwTextOverride is one interface text that an admin replaced without a new
// deploy. The frontend keeps its texts in i18n files keyed by the English
// source text; a row here wins over the file for one language and one key.
// The admin agent proposes the replacement and an admin applies it with one
// click, so every value is checked before it is stored: the placeholders must
// be the ones of the key, the length is bounded and no markup comes in.

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	HwTextOverrideMaxKeyLen   = 400
	HwTextOverrideMaxValueLen = 1000
)

// HwTextOverrideLanguages are the interface languages of the frontend.
var HwTextOverrideLanguages = []string{"tr", "en", "zhCN", "fr", "ru", "ja", "vi", "zhTW"}

type HwTextOverride struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Lang      string `json:"lang" gorm:"size:16;not null;uniqueIndex:idx_hw_text_override_lang_key"`
	TextKey   string `json:"key" gorm:"size:512;not null;uniqueIndex:idx_hw_text_override_lang_key"`
	Value     string `json:"value" gorm:"type:text;not null"`
	UpdatedBy int    `json:"updated_by"`
	UpdatedAt int64  `json:"updated_at"`
}

var hwPlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

// hwPlaceholders returns the sorted placeholder names of a text.
func hwPlaceholders(text string) []string {
	found := hwPlaceholderPattern.FindAllStringSubmatch(text, -1)
	names := make([]string, 0, len(found))
	for _, match := range found {
		names = append(names, match[1])
	}
	sort.Strings(names)
	return names
}

func hwSameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// HwValidateTextOverride refuses a replacement that could break the screen:
// an unknown language, an empty or oversized text, changed placeholders (the
// page fills them in at run time) or markup the key did not have.
func HwValidateTextOverride(lang, key, value string) error {
	known := false
	for _, candidate := range HwTextOverrideLanguages {
		if candidate == lang {
			known = true
		}
	}
	if !known {
		return errors.New("unknown language")
	}
	if strings.TrimSpace(key) == "" || utf8.RuneCountInString(key) > HwTextOverrideMaxKeyLen {
		return errors.New("invalid text key")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("replacement text is empty")
	}
	if utf8.RuneCountInString(value) > HwTextOverrideMaxValueLen {
		return errors.New("replacement text is too long")
	}
	if !hwSameStrings(hwPlaceholders(key), hwPlaceholders(value)) {
		return errors.New("replacement text must keep the same {{placeholders}} as the original")
	}
	if strings.ContainsAny(value, "<>") && !strings.ContainsAny(key, "<>") {
		return errors.New("replacement text may not contain markup")
	}
	return nil
}

// HwSetTextOverride stores or replaces one override after validating it.
func HwSetTextOverride(db *gorm.DB, lang, key, value string, adminID int, now int64) error {
	if err := HwValidateTextOverride(lang, key, value); err != nil {
		return err
	}
	row := HwTextOverride{Lang: lang, TextKey: key, Value: value, UpdatedBy: adminID, UpdatedAt: now}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "lang"}, {Name: "text_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_by", "updated_at"}),
	}).Create(&row).Error
}

// HwDeleteTextOverride removes one override so the file text applies again.
func HwDeleteTextOverride(db *gorm.DB, lang, key string) (bool, error) {
	result := db.Where("lang = ? AND text_key = ?", lang, key).Delete(&HwTextOverride{})
	return result.RowsAffected > 0, result.Error
}

// HwTextOverridesFor returns the overrides of one language as key -> text.
func HwTextOverridesFor(db *gorm.DB, lang string) (map[string]string, error) {
	var rows []HwTextOverride
	if err := db.Where("lang = ?", lang).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.TextKey] = row.Value
	}
	return out, nil
}

// HwListTextOverrides returns every override, newest first, for the admin.
func HwListTextOverrides(db *gorm.DB) ([]HwTextOverride, error) {
	var rows []HwTextOverride
	err := db.Order("updated_at desc, id desc").Limit(500).Find(&rows).Error
	return rows, err
}
