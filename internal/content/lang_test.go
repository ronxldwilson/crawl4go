package content

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantLang   string
		wantScript string
	}{
		{
			name:       "english text",
			text:       "The quick brown fox jumps over the lazy dog and it was a beautiful day",
			wantLang:   "en",
			wantScript: "latin",
		},
		{
			name:       "spanish text",
			text:       "El rápido zorro marrón salta sobre el perro perezoso y la casa es grande",
			wantLang:   "es",
			wantScript: "latin",
		},
		{
			name:       "french text",
			text:       "Le renard brun rapide saute par-dessus le chien paresseux dans les rues de Paris",
			wantLang:   "fr",
			wantScript: "latin",
		},
		{
			name:       "german text",
			text:       "Der schnelle braune Fuchs springt über den faulen Hund und das ist nicht gut",
			wantLang:   "de",
			wantScript: "latin",
		},
		{
			name:       "japanese text",
			text:       "これは日本語のテスト文です。東京タワーは美しいです。",
			wantLang:   "ja",
			wantScript: "cjk",
		},
		{
			name:       "chinese text",
			text:       "这是一个中文测试文本。北京是中国的首都。",
			wantLang:   "zh",
			wantScript: "cjk",
		},
		{
			name:       "korean text",
			text:       "이것은 한국어 테스트입니다. 서울은 대한민국의 수도입니다.",
			wantLang:   "ko",
			wantScript: "hangul",
		},
		{
			name:       "russian text",
			text:       "Быстрая коричневая лисица перепрыгивает через ленивую собаку",
			wantLang:   "ru",
			wantScript: "cyrillic",
		},
		{
			name:       "arabic text",
			text:       "الثعلب البني السريع يقفز فوق الكلب الكسول",
			wantLang:   "ar",
			wantScript: "arabic",
		},
		{
			name:       "hindi text",
			text:       "तेज भूरी लोमड़ी आलसी कुत्ते के ऊपर कूदती है",
			wantLang:   "hi",
			wantScript: "devanagari",
		},
		{
			name:       "empty text",
			text:       "",
			wantLang:   "unknown",
			wantScript: "unknown",
		},
		{
			name:       "whitespace only",
			text:       "   \t\n  ",
			wantLang:   "unknown",
			wantScript: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectLanguage(tt.text)
			if result.Language != tt.wantLang {
				t.Errorf("DetectLanguage(%q).Language = %q, want %q", tt.name, result.Language, tt.wantLang)
			}
			if result.Script != tt.wantScript {
				t.Errorf("DetectLanguage(%q).Script = %q, want %q", tt.name, result.Script, tt.wantScript)
			}
			if tt.wantLang != "unknown" && result.Confidence <= 0 {
				t.Errorf("DetectLanguage(%q).Confidence = %f, want > 0", tt.name, result.Confidence)
			}
		})
	}
}

func TestDetectScript(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"latin", "Hello world", "latin"},
		{"cyrillic", "Привет мир", "cyrillic"},
		{"cjk", "你好世界", "cjk"},
		{"hangul", "안녕하세요", "hangul"},
		{"arabic", "مرحبا بالعالم", "arabic"},
		{"devanagari", "नमस्ते दुनिया", "devanagari"},
		{"numbers only", "12345", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectScript(tt.text)
			if got != tt.want {
				t.Errorf("detectScript(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestClassifyCJK(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"japanese with hiragana", "これはテストです", "ja"},
		{"japanese with katakana", "テスト", "ja"},
		{"chinese only", "这是测试", "zh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCJK(tt.text)
			if got != tt.want {
				t.Errorf("classifyCJK(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestTokenizeLang(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minCount int
	}{
		{"basic words", "Hello World Testing", 3},
		{"with punctuation", "Hello, world! Testing.", 3},
		{"empty", "", 0},
		{"hyphenated", "well-known fact", 2}, // "well-known" and "fact"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenizeLang(tt.text)
			if len(tokens) < tt.minCount {
				t.Errorf("tokenizeLang(%q) = %d tokens, want >= %d", tt.text, len(tokens), tt.minCount)
			}
		})
	}
}
