package service

import (
	"strings"
	"unicode"
)

// ProfanityFilter checks user-submitted text for offensive words using
// whole-word matching. "grass" will NOT be flagged for containing "ass".
type ProfanityFilter struct {
	blocklist map[string]bool
}

// NewProfanityFilter returns a filter loaded with a default blocklist.
func NewProfanityFilter() *ProfanityFilter {
	words := []string{
		// Slurs & hate speech
		"nigger", "nigga", "faggot", "fag", "dyke", "kike", "chink",
		"spic", "wetback", "gook", "raghead", "towelhead", "tranny",
		"retard", "retarded",

		// Common profanity
		"fuck", "fucker", "fucking", "fucked", "motherfucker",
		"shit", "shitty", "bullshit", "horseshit",
		"ass", "asshole", "arsehole",
		"bitch", "cunt", "dick", "cock", "pussy",
		"damn", "damnit", "goddamn",
		"bastard", "whore", "slut", "skank",
		"piss", "pissed",

		// Sexual / inappropriate
		"porn", "hentai", "dildo", "blowjob", "handjob",
		"cum", "jizz", "tits", "boobs",

		// Violence-adjacent
		"kill", "murder", "rape",
	}

	bl := make(map[string]bool, len(words))
	for _, w := range words {
		bl[w] = true
	}
	return &ProfanityFilter{blocklist: bl}
}

// IsClean returns true if text contains no offensive whole words.
// It tokenizes on whitespace and non-letter/digit boundaries, then
// checks each token against the blocklist as an exact match.
func (f *ProfanityFilter) IsClean(text string) bool {
	for _, word := range tokenize(text) {
		if f.blocklist[word] {
			return false
		}
	}
	return true
}

// OffendingWord returns the first blocked word found, or "" if clean.
func (f *ProfanityFilter) OffendingWord(text string) string {
	for _, word := range tokenize(text) {
		if f.blocklist[word] {
			return word
		}
	}
	return ""
}

// tokenize splits text into lowercase alphabetic tokens.
// Non-letter characters act as separators, so "kick-ass" → ["kick", "ass"]
// but "grass" → ["grass"] (single token, won't match "ass").
func tokenize(s string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
