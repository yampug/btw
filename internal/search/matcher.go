package search

import (
	"strings"
	"unicode"

	"github.com/bob/boomerang/internal/model"
)

// MatchResult holds the outcome of matching a query against a candidate.
type MatchResult struct {
	Matched bool
	Score   int
	Ranges  []model.MatchRange
}

// FuzzyMatch matches query against candidate using IntelliJ-style matching.
// An empty or whitespace-only query matches everything with score 0.
// Uppercase characters in the query enforce case at that position.
func FuzzyMatch(query, candidate string) MatchResult {
	query = strings.TrimSpace(query)
	if query == "" {
		return MatchResult{Matched: true, Score: 0}
	}

	qRunes := []rune(query)
	cRunes := []rune(candidate)

	// 1. Exact match (case-insensitive).
	if strings.EqualFold(query, candidate) {
		return MatchResult{
			Matched: true,
			Score:   1000,
			Ranges:  []model.MatchRange{{Start: 0, End: len(cRunes)}},
		}
	}

	// 2. Prefix match (case-insensitive).
	if len(cRunes) >= len(qRunes) && strings.EqualFold(string(cRunes[:len(qRunes)]), query) {
		return MatchResult{
			Matched: true,
			Score:   800,
			Ranges:  []model.MatchRange{{Start: 0, End: len(qRunes)}},
		}
	}

	// 3. CamelCase / CamelHump match.
	if r := matchCamelCase(qRunes, cRunes); r.Matched {
		return r
	}

	// 4. Snake_case match.
	if r := matchSnakeCase(qRunes, cRunes); r.Matched {
		return r
	}

	// 5. Substring match (before subsequence — a contiguous hit is worth more).
	if r := matchSubstring(qRunes, cRunes); r.Matched {
		return r
	}

	// 6. Subsequence match.
	if r := matchSubsequence(qRunes, cRunes); r.Matched {
		return r
	}

	return MatchResult{}
}

// isBoundary reports whether position i in runes is a word boundary.
func isBoundary(runes []rune, i int) bool {
	if i == 0 {
		return true
	}
	cur := runes[i]
	prev := runes[i-1]
	// Uppercase after lowercase: fooBar → B is boundary
	if unicode.IsUpper(cur) && unicode.IsLower(prev) {
		return true
	}
	// After separator: foo_bar → b is boundary, foo-bar → b, foo.bar → b
	if prev == '_' || prev == '-' || prev == '.' || prev == '/' {
		return true
	}
	// Uppercase followed by lowercase after uppercase run: XMLParser → P is boundary
	if i+1 < len(runes) && unicode.IsUpper(cur) && unicode.IsUpper(prev) && unicode.IsLower(runes[i+1]) {
		return true
	}
	return false
}

// charMatch checks if query rune q matches candidate rune c.
// If q is uppercase, it requires exact case match.
// If q is lowercase, it matches case-insensitively.
func charMatch(q, c rune) bool {
	if unicode.IsUpper(q) {
		return q == c
	}
	return unicode.ToLower(q) == unicode.ToLower(c)
}

// matchCamelCase tries to match each query char to a word-boundary char in the candidate.
func matchCamelCase(qRunes, cRunes []rune) MatchResult {
	// Collect boundary positions.
	var boundaries []int
	for i := range cRunes {
		if isBoundary(cRunes, i) {
			boundaries = append(boundaries, i)
		}
	}

	if len(boundaries) < len(qRunes) {
		return MatchResult{}
	}

	// Try to match query chars to boundary chars in order.
	var ranges []model.MatchRange
	qi := 0
	for _, bi := range boundaries {
		if qi >= len(qRunes) {
			break
		}
		if charMatch(qRunes[qi], cRunes[bi]) {
			ranges = append(ranges, model.MatchRange{Start: bi, End: bi + 1})
			qi++
		}
	}

	if qi == len(qRunes) {
		return MatchResult{
			Matched: true,
			Score:   600,
			Ranges:  ranges,
		}
	}

	// Fallback: match boundary chars, but allow consuming non-boundary chars
	// contiguously after a boundary match (e.g. "grs" → getResponseStatus).
	ranges = nil
	qi = 0
	ci := 0
	for qi < len(qRunes) && ci < len(cRunes) {
		if isBoundary(cRunes, ci) && charMatch(qRunes[qi], cRunes[ci]) {
			start := ci
			qi++
			ci++
			// Consume contiguous matches after the boundary.
			for qi < len(qRunes) && ci < len(cRunes) && !isBoundary(cRunes, ci) && charMatch(qRunes[qi], cRunes[ci]) {
				qi++
				ci++
			}
			ranges = append(ranges, model.MatchRange{Start: start, End: ci})
		} else {
			ci++
		}
	}

	if qi == len(qRunes) {
		return MatchResult{
			Matched: true,
			Score:   600,
			Ranges:  ranges,
		}
	}

	return MatchResult{}
}

// matchSnakeCase tries to match query chars against segment starts separated by _.
func matchSnakeCase(qRunes, cRunes []rune) MatchResult {
	candidate := string(cRunes)
	if !strings.Contains(candidate, "_") {
		return MatchResult{}
	}

	// Build segment start positions.
	var segStarts []int
	segStarts = append(segStarts, 0)
	for i, r := range cRunes {
		if r == '_' && i+1 < len(cRunes) {
			segStarts = append(segStarts, i+1)
		}
	}

	if len(segStarts) < len(qRunes) {
		return MatchResult{}
	}

	// Match each query char to a segment start, allowing contiguous consumption.
	var ranges []model.MatchRange
	qi := 0
	for _, si := range segStarts {
		if qi >= len(qRunes) {
			break
		}
		ci := si
		if charMatch(qRunes[qi], cRunes[ci]) {
			start := ci
			qi++
			ci++
			for qi < len(qRunes) && ci < len(cRunes) && cRunes[ci] != '_' && charMatch(qRunes[qi], cRunes[ci]) {
				qi++
				ci++
			}
			ranges = append(ranges, model.MatchRange{Start: start, End: ci})
		}
	}

	if qi == len(qRunes) {
		return MatchResult{
			Matched: true,
			Score:   550,
			Ranges:  ranges,
		}
	}
	return MatchResult{}
}

// matchSubstring checks if query is a contiguous substring of candidate (case-insensitive).
func matchSubstring(qRunes, cRunes []rune) MatchResult {
	lowerC := []rune(strings.ToLower(string(cRunes)))
	lowerQ := []rune(strings.ToLower(string(qRunes)))

	for i := 0; i <= len(lowerC)-len(lowerQ); i++ {
		found := true
		for j := range lowerQ {
			if !charMatch(qRunes[j], cRunes[i+j]) {
				found = false
				break
			}
		}
		if found {
			return MatchResult{
				Matched: true,
				Score:   500,
				Ranges:  []model.MatchRange{{Start: i, End: i + len(lowerQ)}},
			}
		}
	}
	return MatchResult{}
}

// matchSubsequence checks if query chars appear in order in candidate.
func matchSubsequence(qRunes, cRunes []rune) MatchResult {
	var ranges []model.MatchRange
	qi := 0
	for ci := 0; ci < len(cRunes) && qi < len(qRunes); ci++ {
		if charMatch(qRunes[qi], cRunes[ci]) {
			ranges = append(ranges, model.MatchRange{Start: ci, End: ci + 1})
			qi++
		}
	}

	if qi == len(qRunes) {
		// Score based on compactness.
		spread := ranges[len(ranges)-1].End - ranges[0].Start
		score := 300 - spread
		if score < 1 {
			score = 1
		}
		return MatchResult{
			Matched: true,
			Score:   score,
			Ranges:  ranges,
		}
	}
	return MatchResult{}
}
