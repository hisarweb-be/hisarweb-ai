package service

// The assistant's fixed answers. A question that matches an entry costs no
// model call: the answer is built from the code (the sold catalog, the
// discount), never retyped, so it cannot drift from the real offer. The order of
// the chips follows how often an entry was asked; a new question is only
// counted, and an entry is added by a developer with a checked answer.

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// HwAssistantLanguages are the languages the fixed answers exist in.
var HwAssistantLanguages = []string{"tr", "en", "nl"}

// HwAssistantIdentity is the answer to "who are you". It never names a model.
var HwAssistantIdentity = map[string]string{
	"tr": "Ben HisarWeb AI'nin sohbet çalışanıyım. Claude API ile çalışıyorum ve api.hisarweb.be hakkındaki sorularınızı yanıtlıyorum. Hangi konuda yardımcı olabilirim?",
	"en": "I am the chat employee of HisarWeb AI. I run on the Claude API and answer questions about api.hisarweb.be. What can I help you with?",
	"nl": "Ik ben de chatmedewerker van HisarWeb AI. Ik draai op de Claude API en beantwoord vragen over api.hisarweb.be. Waarmee kan ik je helpen?",
}

// HwAssistantFAQ is one fixed question with its keywords and answers. The
// keywords carry their language, so a match also tells the language the
// question was written in and the answer comes in that language.
type HwAssistantFAQ struct {
	ID       string
	Question map[string]string
	Keywords map[string][]string
	Answer   func(lang string) string
}

func hwAssistantPick(lang string, tr, en, nl string) string {
	switch lang {
	case "en":
		return en
	case "nl":
		return nl
	}
	return tr
}

// hwAssistantModelLines lists every sold model with the price we bill per
// million tokens (the list price minus the discount), read from the catalog.
func hwAssistantModelLines() []string {
	lines := make([]string, 0, len(hwSoldModels))
	for _, sold := range hwSoldModels {
		in := float64(sold.InputMicroUsd) / 1e6 * (100 - HwListPriceDiscountPercent) / 100
		out := float64(sold.OutputMicroUsd) / 1e6 * (100 - HwListPriceDiscountPercent) / 100
		lines = append(lines, fmt.Sprintf("%s: $%.2f / $%.2f", sold.Name, in, out))
	}
	return lines
}

var hwAssistantFAQs = []HwAssistantFAQ{
	{
		ID:       "identity",
		Question: map[string]string{"tr": "Sen kimsin?", "en": "Who are you?", "nl": "Wie ben je?"},
		Keywords: map[string][]string{
			"tr": {"sen kimsin", "kimsin", "hangi model", "hangi yapay zeka"},
			"en": {"who are you", "what are you", "which model"},
			"nl": {"wie ben je", "wie ben jij", "welk model", "welke ai"},
		},
		Answer: func(lang string) string { return HwAssistantIdentity[hwAssistantLang(lang)] },
	},
	{
		ID:       "models",
		Question: map[string]string{"tr": "Hangi modelleri sunuyorsunuz?", "en": "Which models do you offer?", "nl": "Welke modellen bieden jullie aan?"},
		Keywords: map[string][]string{
			"tr": {"hangi modeller", "modeller"},
			"en": {"which models", "models do you"},
			"nl": {"welke modellen", "modellen"},
		},
		Answer: func(lang string) string {
			list := strings.Join(hwAssistantModelLines(), "\n")
			return hwAssistantPick(lang,
				"Sunduğumuz modeller (milyon token başına giriş / çıkış fiyatı):\n"+list+"\nHangisini denemek istersiniz?",
				"Our models (price per million tokens, input / output):\n"+list+"\nWhich one would you like to try?",
				"Onze modellen (prijs per miljoen tokens, invoer / uitvoer):\n"+list+"\nWelke wil je proberen?")
		},
	},
	{
		ID:       "price",
		Question: map[string]string{"tr": "Fiyatlar nasıl?", "en": "How much does it cost?", "nl": "Wat kost het?"},
		Keywords: map[string][]string{
			"tr": {"fiyat", "ucret", "kac para", "indirim"},
			"en": {"price", "pricing", "cost", "how much", "discount"},
			"nl": {"prijs", "kost", "tarief", "korting"},
		},
		Answer: func(lang string) string {
			list := strings.Join(hwAssistantModelLines(), "\n")
			return hwAssistantPick(lang,
				fmt.Sprintf("Her modeli resmi liste fiyatının %%%d altında faturalandırıyoruz. Milyon token başına giriş / çıkış:\n%s\nKaç token kullanmayı planlıyorsunuz?", HwListPriceDiscountPercent, list),
				fmt.Sprintf("We bill every model %d%% below the official list price. Per million tokens, input / output:\n%s\nHow many tokens do you plan to use?", HwListPriceDiscountPercent, list),
				fmt.Sprintf("We rekenen elk model %d%% onder de officiële lijstprijs af. Per miljoen tokens, invoer / uitvoer:\n%s\nHoeveel tokens wil je gebruiken?", HwListPriceDiscountPercent, list))
		},
	},
	{
		ID:       "api",
		Question: map[string]string{"tr": "API'yi nasıl kullanırım?", "en": "How do I use the API?", "nl": "Hoe gebruik ik de API?"},
		Keywords: map[string][]string{
			"tr": {"api", "nasil kullan", "anahtar"},
			"en": {"api", "how do i use", "how to use", "api key", "openai", "base url", "endpoint"},
			"nl": {"api", "hoe gebruik", "sleutel"},
		},
		Answer: func(lang string) string {
			return hwAssistantPick(lang,
				"Hesabınızdan bir API anahtarı oluşturun. İstekleri https://api.hisarweb.be/v1 adresine OpenAI uyumlu biçimde gönderin ve model adı olarak listemizdeki adlardan birini yazın. Hangi dilde kod yazıyorsunuz?",
				"Create an API key in your account. Send OpenAI-compatible requests to https://api.hisarweb.be/v1 and use one of the model names from our list. Which language do you code in?",
				"Maak in je account een API-sleutel aan. Stuur OpenAI-compatibele verzoeken naar https://api.hisarweb.be/v1 en gebruik een modelnaam uit onze lijst. In welke taal programmeer je?")
		},
	},
	{
		ID:       "currency",
		Question: map[string]string{"tr": "Hangi para birimleriyle ödeyebilirim?", "en": "Which currencies can I pay in?", "nl": "In welke munten kan ik betalen?"},
		Keywords: map[string][]string{
			"tr": {"para birimi", "lira", "dolar", "odeme"},
			"en": {"currency", "currencies", "dollar", "payment"},
			"nl": {"munt", "valuta", "euro", "betalen", "betaling"},
		},
		Answer: func(lang string) string {
			return hwAssistantPick(lang,
				"TL, euro ve dolar ile ödeme yapabilirsiniz. Satın aldığınız krediyi tam olarak alırsınız; kur farkı maliyetini biz karşılarız. Hangi para birimini seçersiniz?",
				"You can pay in TRY, EUR and USD. You receive the full credit you buy; we carry the currency conversion cost. Which currency do you prefer?",
				"Je kunt betalen in TRY, EUR en USD. Je krijgt het volledige tegoed dat je koopt; de omrekenkosten zijn voor ons. Welke munt kies je?")
		},
	},
}

func hwAssistantLang(lang string) string {
	for _, known := range HwAssistantLanguages {
		if lang == known {
			return lang
		}
	}
	return "tr"
}

var hwAssistantFold = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// HwAssistantFold lower-cases a question and removes accents, so "Şu ışık" and
// "su isik" match the same keyword. The Turkish dotless i becomes a plain i.
func HwAssistantFold(text string) string {
	text = strings.NewReplacer("ı", "i", "İ", "i", "I", "i").Replace(text)
	folded, _, err := transform.String(hwAssistantFold, strings.ToLower(text))
	if err != nil {
		folded = strings.ToLower(text)
	}
	return strings.Join(strings.Fields(folded), " ")
}

// hwAssistantLanguageHints are everyday words that betray the language a
// question is written in, independent of any FAQ keyword. They are folded the
// same way as the question, so the Turkish ı is a plain i here.
var hwAssistantLanguageHints = map[string][]string{
	"tr": {"nasil", "yapabilirim", "kullanirim", "kullanabilirim", "icin", "hangi", "nedir", "beni", "kac", "nerede"},
	"en": {"how", "can", "the", "you", "what", "my", "do", "does", "use", "install"},
	"nl": {"hoe", "kan", "ik", "dit", "die", "op", "mijn", "wat", "een", "je", "jullie", "gebruik", "installeren"},
}

// hwAssistantQuestionLanguage guesses the language of the question from its
// own words. It decides only when one language clearly leads, so a bare
// keyword ("api") stays with the panel preference.
func hwAssistantQuestionLanguage(folded string) string {
	best, bestHits := "", 0
	tied := false
	for _, lang := range HwAssistantLanguages {
		hits := 0
		for _, word := range strings.Fields(folded) {
			for _, hint := range hwAssistantLanguageHints[lang] {
				if word == hint {
					hits++
					break
				}
			}
		}
		if hits > bestHits {
			best, bestHits, tied = lang, hits, false
		} else if hits == bestHits && hits > 0 {
			tied = true
		}
	}
	if bestHits == 0 || tied {
		return ""
	}
	return best
}

// HwAssistantMatch returns the entry that best matches the question and the
// language the answer must be written in, or nil. The keyword that is longest
// wins, so "sen kimsin" beats a single word. The answer language is the
// language the question itself is written in whenever its own everyday words
// betray it: a Turkish sentence with the English loan word "api key" is
// answered in Turkish. Only a question without any language hints falls back
// to the language of the matched keyword, and a keyword that lives in several
// languages ("api") then counts for the preferred panel language.
func HwAssistantMatch(question, preferred string) (*HwAssistantFAQ, string) {
	folded := HwAssistantFold(question)
	if folded == "" {
		return nil, ""
	}
	order := []string{hwAssistantLang(preferred)}
	for _, known := range HwAssistantLanguages {
		if known != order[0] {
			order = append(order, known)
		}
	}
	var best *HwAssistantFAQ
	bestLang := ""
	bestScore := 0
	for index := range hwAssistantFAQs {
		for _, lang := range order {
			for _, keyword := range hwAssistantFAQs[index].Keywords[lang] {
				if strings.Contains(folded, HwAssistantFold(keyword)) && len(keyword) > bestScore {
					best, bestLang, bestScore = &hwAssistantFAQs[index], lang, len(keyword)
				}
			}
		}
	}
	if best != nil {
		if questionLang := hwAssistantQuestionLanguage(folded); questionLang != "" {
			bestLang = questionLang
		}
	}
	return best, bestLang
}

// HwAssistantFAQView is one chip: the question and its fixed answer.
type HwAssistantFAQView struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// HwAssistantFAQList returns the entries in the language, most asked first.
// hits maps an entry id to how often it was asked.
func HwAssistantFAQList(lang string, hits map[string]int) []HwAssistantFAQView {
	lang = hwAssistantLang(lang)
	views := make([]HwAssistantFAQView, 0, len(hwAssistantFAQs))
	for _, entry := range hwAssistantFAQs {
		views = append(views, HwAssistantFAQView{ID: entry.ID, Question: entry.Question[lang], Answer: entry.Answer(lang)})
	}
	sort.SliceStable(views, func(a, b int) bool { return hits[views[a].ID] > hits[views[b].ID] })
	return views
}

// HwAssistantAnswerFor returns the fixed answer for a question, if any. The
// answer is written in the language of the question itself — the language its
// own words betray, whatever keyword matched it — and only a question without
// any language hints is answered in the language of the matched keyword, so
// "sen kimsin" is answered in Turkish everywhere and a Turkish sentence with
// "api key" in it is answered in Turkish too.
func HwAssistantAnswerFor(question, lang string) (id, answer string, ok bool) {
	entry, matched := HwAssistantMatch(question, lang)
	if entry == nil {
		return "", "", false
	}
	return entry.ID, entry.Answer(matched), true
}
