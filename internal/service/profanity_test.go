package service

import "testing"

// ── IsClean ─────────────────────────────────────────────────

func TestIsClean_CleanText(t *testing.T) {
	f := NewProfanityFilter()

	clean := []string{
		"Great climb today!",
		"This route is amazing",
		"V5 overhang with crimps",
		"Classic setter move",
		"",
		"Hello world",
		"The grass is green",        // contains "ass" but as substring, should NOT match
		"Classy route setting",      // contains "ass" as substring
		"A mass of holds on the wall", // contains "ass" as substring
		"Compass directions",        // contains "ass" as substring
		"Assassin",                  // contains "ass" as substring
		"Discussion forum",          // contains no profanity
		"Cocktail bar",             // contains "cock" as substring but as part of "cocktail"
		"Scunthorpe problem",       // contains "cunt" as substring
		"Lake Titticaca",           // contains "tit" as substring
	}

	for _, text := range clean {
		if !f.IsClean(text) {
			t.Errorf("expected %q to be clean", text)
		}
	}
}

func TestIsClean_DirtyText(t *testing.T) {
	f := NewProfanityFilter()

	dirty := []string{
		"This climb is shit",
		"What the fuck",
		"That's a damn hard route",
		"Shit grade",
		"fuck this wall",
	}

	for _, text := range dirty {
		if f.IsClean(text) {
			t.Errorf("expected %q to be dirty", text)
		}
	}
}

func TestIsClean_CaseInsensitive(t *testing.T) {
	f := NewProfanityFilter()

	cases := []string{
		"SHIT", "Shit", "sHiT", "FUCK", "Fuck",
	}
	for _, text := range cases {
		if f.IsClean(text) {
			t.Errorf("expected %q to be caught (case-insensitive)", text)
		}
	}
}

func TestIsClean_PunctuationSeparated(t *testing.T) {
	f := NewProfanityFilter()

	// Words separated by punctuation should be tokenized and caught
	dirty := []string{
		"kick-ass route",       // "ass" is a separate token after hyphen
		"that's shit!",         // "shit" followed by punctuation
		"WTF? fuck this",       // question mark separates
		"route... damn... hard", // dots as separators
	}

	for _, text := range dirty {
		if f.IsClean(text) {
			t.Errorf("expected %q to be dirty (punctuation-separated)", text)
		}
	}
}

func TestIsClean_NumbersInTokens(t *testing.T) {
	f := NewProfanityFilter()

	// Tokens with digits should not match blocklist
	if !f.IsClean("V5 is a gr8 grade") {
		t.Error("alphanumeric tokens should not match blocklist")
	}
}

// ── OffendingWord ───────────────────────────────────────────

func TestOffendingWord_ReturnsFirstMatch(t *testing.T) {
	f := NewProfanityFilter()

	word := f.OffendingWord("This is shit and damn bad")
	if word != "shit" {
		t.Errorf("OffendingWord = %q, want %q", word, "shit")
	}
}

func TestOffendingWord_CleanText(t *testing.T) {
	f := NewProfanityFilter()

	word := f.OffendingWord("Great route!")
	if word != "" {
		t.Errorf("OffendingWord on clean text = %q, want empty", word)
	}
}

func TestOffendingWord_EmptyString(t *testing.T) {
	f := NewProfanityFilter()

	word := f.OffendingWord("")
	if word != "" {
		t.Errorf("OffendingWord on empty = %q, want empty", word)
	}
}

// ── tokenize ────────────────────────────────────────────────

func TestTokenize(t *testing.T) {
	tests := []struct {
		input  string
		expect []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"kick-ass route", []string{"kick", "ass", "route"}},
		{"UPPER case", []string{"upper", "case"}},
		{"multiple   spaces", []string{"multiple", "spaces"}},
		{"", nil},
		{"   ", nil},
		{"a,b;c.d", []string{"a", "b", "c", "d"}},
		{"hello123world", []string{"hello123world"}},
		{"V5!", []string{"v5"}},
		{"emoji 🎉 test", []string{"emoji", "test"}},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := tokenize(tc.input)
			if len(got) != len(tc.expect) {
				t.Fatalf("tokenize(%q) len = %d, want %d (%v)", tc.input, len(got), len(tc.expect), got)
			}
			for i := range got {
				if got[i] != tc.expect[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.expect[i])
				}
			}
		})
	}
}

// ── Blocklist coverage ──────────────────────────────────────

func TestBlocklist_ContainsExpectedWords(t *testing.T) {
	f := NewProfanityFilter()

	// Sample check — not exhaustive, just verify the blocklist loaded
	expectedBlocked := []string{
		"fuck", "shit", "ass", "bitch", "damn",
		"nigger", "faggot", "retard", "cunt",
		"porn", "rape", "kill",
	}

	for _, word := range expectedBlocked {
		if f.IsClean(word) {
			t.Errorf("expected %q to be in blocklist", word)
		}
	}
}

func TestBlocklist_DoesNotBlockInnocentWords(t *testing.T) {
	f := NewProfanityFilter()

	// Words that contain blocked substrings but are innocent
	innocent := []string{
		"grass", "class", "classic", "mass", "compass",
		"assistant", "cocktail", "peacock",
		"scunthorpe", "basement",
		"skill", "killed", // "kill" is blocked but "killed" is a single token...
		"grape", "drape",
	}

	for _, word := range innocent {
		if !f.IsClean(word) {
			offending := f.OffendingWord(word)
			t.Errorf("expected %q to be clean (caught: %q)", word, offending)
		}
	}
}

// ── Benchmark ───────────────────────────────────────────────

func BenchmarkIsClean_Clean(b *testing.B) {
	f := NewProfanityFilter()
	text := "This is a really fun V5 boulder problem with crimps and a tricky heel hook on the overhang"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.IsClean(text)
	}
}

func BenchmarkIsClean_Dirty(b *testing.B) {
	f := NewProfanityFilter()
	text := "This route is complete shit and the setter can go fuck themselves"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.IsClean(text)
	}
}
