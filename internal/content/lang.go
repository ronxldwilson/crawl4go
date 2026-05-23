package content

import (
	"strings"
	"unicode"
)

// LangResult holds the detected language, confidence score, and script family.
type LangResult struct {
	Language   string  `json:"language"`
	Confidence float64 `json:"confidence"`
	Script     string  `json:"script"`
}

// DetectLanguage performs statistical language detection on the given text
// using Unicode script analysis and common-word frequency matching.
// No external dependencies are required.
func DetectLanguage(text string) LangResult {
	if len(strings.TrimSpace(text)) == 0 {
		return LangResult{Language: "unknown", Confidence: 0, Script: "unknown"}
	}

	script := detectScript(text)

	// For non-Latin scripts, the script itself identifies the language family.
	switch script {
	case "cjk":
		lang := classifyCJK(text)
		return LangResult{Language: lang, Confidence: 0.8, Script: script}
	case "cyrillic":
		return LangResult{Language: "ru", Confidence: 0.75, Script: script}
	case "arabic":
		return LangResult{Language: "ar", Confidence: 0.75, Script: script}
	case "devanagari":
		return LangResult{Language: "hi", Confidence: 0.75, Script: script}
	case "hangul":
		return LangResult{Language: "ko", Confidence: 0.85, Script: script}
	}

	// Latin script: use word-frequency matching.
	lang, conf := detectLatinLanguage(text)
	return LangResult{Language: lang, Confidence: conf, Script: "latin"}
}

// detectScript examines character frequencies to determine the dominant script.
func detectScript(text string) string {
	var latin, cyrillic, cjk, arabic, devanagari, hangul, total int

	for _, r := range text {
		if unicode.IsLetter(r) {
			total++
			switch {
			case r >= 0x0400 && r <= 0x04FF:
				cyrillic++
			case r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF:
				cjk++
			case r >= 0x3040 && r <= 0x30FF: // Hiragana + Katakana
				cjk++
			case r >= 0xAC00 && r <= 0xD7AF:
				hangul++
			case r >= 0x0600 && r <= 0x06FF:
				arabic++
			case r >= 0x0900 && r <= 0x097F:
				devanagari++
			case r >= 0x0041 && r <= 0x024F: // Basic Latin + Latin Extended
				latin++
			}
		}
	}

	if total == 0 {
		return "unknown"
	}

	type scored struct {
		name  string
		count int
	}
	best := scored{"latin", latin}
	for _, s := range []scored{
		{"cyrillic", cyrillic},
		{"cjk", cjk},
		{"arabic", arabic},
		{"devanagari", devanagari},
		{"hangul", hangul},
	} {
		if s.count > best.count {
			best = s
		}
	}

	if float64(best.count)/float64(total) < 0.3 {
		return "latin" // default fallback
	}
	return best.name
}

// classifyCJK distinguishes Chinese from Japanese by checking for
// Hiragana/Katakana presence (unique to Japanese).
func classifyCJK(text string) string {
	for _, r := range text {
		if r >= 0x3040 && r <= 0x309F { // Hiragana
			return "ja"
		}
		if r >= 0x30A0 && r <= 0x30FF { // Katakana
			return "ja"
		}
	}
	return "zh"
}

// detectLatinLanguage matches the text against common words from Latin-script
// languages and returns the best match with a confidence score.
func detectLatinLanguage(text string) (string, float64) {
	words := tokenizeLang(text)
	if len(words) == 0 {
		return "unknown", 0
	}

	wordSet := make(map[string]struct{}, len(words))
	for _, w := range words {
		wordSet[w] = struct{}{}
	}

	type langScore struct {
		code  string
		score int
	}

	var best langScore
	for code, commonWords := range latinLangWords {
		score := 0
		for _, cw := range commonWords {
			if _, ok := wordSet[cw]; ok {
				score++
			}
		}
		if score > best.score {
			best = langScore{code, score}
		}
	}

	if best.score == 0 {
		return "unknown", 0
	}

	// Confidence is the fraction of common words found, capped at 1.0.
	totalCommon := len(latinLangWords[best.code])
	confidence := float64(best.score) / float64(totalCommon)
	if confidence > 1.0 {
		confidence = 1.0
	}
	return best.code, confidence
}

// tokenize lowercases text and splits on whitespace/punctuation, returning
// unique words.
func tokenizeLang(text string) []string {
	lower := strings.ToLower(text)
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.Is(unicode.Hyphen, r)
	})
	return fields
}

// latinLangWords maps language codes to their 20 most common function words.
// These are high-frequency, language-specific stop words that reliably
// distinguish languages.
var latinLangWords = map[string][]string{
	"en": {"the", "be", "to", "of", "and", "a", "in", "that", "have", "i",
		"it", "for", "not", "on", "with", "he", "as", "you", "do", "at"},
	"es": {"de", "la", "que", "el", "en", "y", "a", "los", "se", "del",
		"las", "un", "por", "con", "no", "una", "su", "para", "es", "al"},
	"fr": {"de", "la", "le", "et", "les", "des", "en", "un", "du", "une",
		"que", "est", "dans", "qui", "au", "ce", "il", "pas", "plus", "sur"},
	"de": {"der", "die", "und", "in", "den", "von", "zu", "das", "mit", "sich",
		"des", "auf", "ist", "ein", "dem", "nicht", "eine", "als", "auch", "es"},
	"pt": {"de", "a", "o", "que", "e", "do", "da", "em", "um", "para",
		"com", "uma", "os", "no", "se", "na", "por", "mais", "as", "dos"},
	"it": {"di", "e", "il", "la", "in", "che", "un", "per", "del", "una",
		"dei", "le", "della", "con", "si", "da", "al", "lo", "sono", "gli"},
	"nl": {"de", "het", "een", "van", "en", "in", "is", "dat", "op", "te",
		"voor", "zijn", "met", "die", "niet", "aan", "er", "maar", "ook", "als"},
	"ro": {"de", "in", "la", "si", "cu", "din", "pe", "un", "ce", "mai",
		"care", "este", "nu", "se", "o", "pentru", "sau", "lui", "sunt", "dar"},
	"sv": {"och", "i", "att", "en", "det", "som", "har", "med", "av", "den",
		"till", "var", "inte", "ett", "om", "jag", "han", "hade", "vid", "kan"},
	"da": {"og", "i", "at", "en", "det", "er", "til", "den", "af", "som",
		"med", "har", "han", "var", "jeg", "ikke", "et", "hun", "blev", "kan"},
	"no": {"og", "i", "det", "er", "en", "til", "som", "har", "med", "av",
		"den", "at", "ikke", "han", "var", "jeg", "et", "hun", "ble", "kan"},
	"pl": {"i", "w", "na", "nie", "to", "jest", "sie", "z", "do", "jak",
		"ale", "ze", "co", "tak", "za", "od", "po", "ich", "ten", "tego"},
}
