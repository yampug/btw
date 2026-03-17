package search

import (
	"strings"

	"github.com/bob/boomerang/internal/model"
)

// ScoreParams holds contextual information used to refine a match score.
type ScoreParams struct {
	RelPath   string // relative path (for depth penalty)
	Name      string // filename (for length tie-breaking)
	Recent    bool   // recently opened file
	Frequent  bool   // frequently selected file
}

// Score takes a base MatchResult from FuzzyMatch and applies contextual
// scoring factors, returning a final score suitable for ranking.
// The returned score is deterministic for identical inputs.
func Score(mr MatchResult, params ScoreParams) int {
	if !mr.Matched {
		return 0
	}

	score := mr.Score

	// Consecutive character bonus: +50 per consecutive pair within ranges.
	score += consecutiveBonus(mr.Ranges)

	// Word-boundary alignment: +100 per matched char that sits on a boundary.
	score += boundaryAlignmentBonus(mr.Ranges, params.Name)

	// Gap penalty: -10 per gap character between matched ranges.
	score += gapPenalty(mr.Ranges)

	// Position penalty: -5 × start position of first match.
	score += positionPenalty(mr.Ranges)

	// File depth penalty: -2 × directory depth.
	score += depthPenalty(params.RelPath)

	// Recency boost.
	if params.Recent {
		score += 200
	}

	// Frequency boost.
	if params.Frequent {
		score += 150
	}

	if score < 1 && mr.Matched {
		score = 1
	}
	return score
}

// consecutiveBonus awards +50 for each pair of consecutively matched characters.
func consecutiveBonus(ranges []model.MatchRange) int {
	bonus := 0
	for _, r := range ranges {
		span := r.End - r.Start
		if span > 1 {
			bonus += (span - 1) * 50
		}
	}
	return bonus
}

// boundaryAlignmentBonus awards +100 per matched character on a word boundary.
func boundaryAlignmentBonus(ranges []model.MatchRange, name string) int {
	if name == "" {
		return 0
	}
	runes := []rune(name)
	bonus := 0
	for _, r := range ranges {
		for i := r.Start; i < r.End && i < len(runes); i++ {
			if isBoundary(runes, i) {
				bonus += 100
			}
		}
	}
	return bonus
}

// gapPenalty applies -10 per character gap between consecutive match ranges.
func gapPenalty(ranges []model.MatchRange) int {
	if len(ranges) <= 1 {
		return 0
	}
	penalty := 0
	for i := 1; i < len(ranges); i++ {
		gap := ranges[i].Start - ranges[i-1].End
		if gap > 0 {
			penalty -= gap * 10
		}
	}
	return penalty
}

// positionPenalty applies -5 × the start position of the first match.
func positionPenalty(ranges []model.MatchRange) int {
	if len(ranges) == 0 {
		return 0
	}
	return -5 * ranges[0].Start
}

// depthPenalty applies -2 per directory level in the relative path.
func depthPenalty(relPath string) int {
	if relPath == "" {
		return 0
	}
	depth := strings.Count(relPath, "/")
	return -2 * depth
}

// RankResults sorts results by score descending with deterministic tie-breaking:
// 1. Higher score first
// 2. Shorter name first
// 3. Shallower path first
// 4. Alphabetical order
func RankResults(results []model.SearchResult) {
	n := len(results)
	if n <= 1 {
		return
	}
	// Use insertion sort for stability + simplicity on typically small result sets.
	for i := 1; i < n; i++ {
		for j := i; j > 0 && lessResult(results[j], results[j-1]); j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}

func lessResult(a, b model.SearchResult) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	la, lb := len([]rune(a.Name)), len([]rune(b.Name))
	if la != lb {
		return la < lb
	}
	da, db := strings.Count(a.Detail, "/"), strings.Count(b.Detail, "/")
	if da != db {
		return da < db
	}
	return a.Name < b.Name
}
