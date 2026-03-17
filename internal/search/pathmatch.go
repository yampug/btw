package search

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/bob/boomerang/internal/model"
)

var lineNumRe = regexp.MustCompile(`:(\d+)$`)

// ParsedQuery holds a query after extracting `:N` line-number suffix.
type ParsedQuery struct {
	Query   string // query text without `:N`
	LineNum int    // parsed line number; 0 if absent
	IsPath  bool   // true if query contains `/`
}

// ParseQuery extracts an optional `:N` suffix and detects path queries.
func ParseQuery(raw string) ParsedQuery {
	pq := ParsedQuery{}
	raw = strings.TrimSpace(raw)

	if m := lineNumRe.FindStringSubmatch(raw); m != nil {
		pq.LineNum, _ = strconv.Atoi(m[1])
		raw = raw[:strings.LastIndex(raw, ":")]
	}

	pq.Query = raw
	pq.IsPath = strings.Contains(raw, "/")
	return pq
}

// PathMatch matches a path query against a candidate relative path.
// The query is split on `/` into segments and each segment is fuzzy-matched
// against consecutive path components of the candidate (allowing gaps).
// Returns a combined MatchResult with ranges mapped to the full relPath.
func PathMatch(query, relPath string) MatchResult {
	qSegments := strings.Split(query, "/")
	cSegments := strings.Split(relPath, "/")

	// Remove empty segments from query (e.g. trailing slash).
	var qSegs []string
	for _, s := range qSegments {
		if s != "" {
			qSegs = append(qSegs, s)
		}
	}
	if len(qSegs) == 0 {
		return MatchResult{Matched: true, Score: 0}
	}

	// Match query segments against candidate segments in order (allowing gaps).
	var allRanges []model.MatchRange
	totalScore := 0
	qi := 0
	charOffset := 0 // running character offset within the full relPath

	for ci := 0; ci < len(cSegments) && qi < len(qSegs); ci++ {
		seg := cSegments[ci]
		mr := FuzzyMatch(qSegs[qi], seg)
		if mr.Matched && mr.Score > 0 {
			// Offset the ranges to map to the full relPath position.
			for _, r := range mr.Ranges {
				allRanges = append(allRanges, model.MatchRange{
					Start: charOffset + r.Start,
					End:   charOffset + r.End,
				})
			}
			totalScore += mr.Score
			qi++
		}
		charOffset += len([]rune(seg)) + 1 // +1 for the `/` separator
	}

	if qi < len(qSegs) {
		return MatchResult{}
	}

	// Average score across matched segments, with a bonus for matching more segments.
	avgScore := totalScore / len(qSegs)
	segBonus := len(qSegs) * 20

	return MatchResult{
		Matched: true,
		Score:   avgScore + segBonus,
		Ranges:  allRanges,
	}
}
