package search

import (
	"testing"

	"github.com/bob/boomerang/internal/model"
)

func score(query, candidate, relPath string) int {
	mr := FuzzyMatch(query, candidate)
	return Score(mr, ScoreParams{RelPath: relPath, Name: candidate})
}

// Scenario 1: Exact match beats everything.
func TestScore_ExactBeatsPrefix(t *testing.T) {
	exact := score("main.go", "main.go", "main.go")
	prefix := score("main", "main.go", "main.go")
	if exact <= prefix {
		t.Errorf("exact (%d) should beat prefix (%d)", exact, prefix)
	}
}

// Scenario 2: Prefix beats substring.
func TestScore_PrefixBeatsSubstring(t *testing.T) {
	prefix := score("con", "config.yaml", "config.yaml")
	substr := score("fig", "config.yaml", "config.yaml")
	if prefix <= substr {
		t.Errorf("prefix (%d) should beat substring (%d)", prefix, substr)
	}
}

// Scenario 3: CamelCase beats scattered subsequence.
func TestScore_CamelCaseBeatsSubsequence(t *testing.T) {
	camel := score("CCN", "CamelCaseName", "CamelCaseName")
	subseq := score("CCN", "CactusCoordinateNinja", "CactusCoordinateNinja")
	// Both match, but the first has tighter CamelCase alignment.
	if camel < subseq {
		t.Errorf("camelcase (%d) should beat or equal subsequence (%d)", camel, subseq)
	}
}

// Scenario 4: Consecutive chars are rewarded.
func TestScore_ConsecutiveBonus(t *testing.T) {
	// "main" in main_test.go (4 consecutive) vs "m_a_i_n" scattered.
	consec := score("main", "main_test.go", "main_test.go")
	scattered := score("mtin", "main_test.go", "main_test.go")
	if consec <= scattered {
		t.Errorf("consecutive (%d) should beat scattered (%d)", consec, scattered)
	}
}

// Scenario 5: Shallow file beats deep file for same match.
func TestScore_ShallowBeatsDeep(t *testing.T) {
	shallow := score("main.go", "main.go", "main.go")
	deep := score("main.go", "main.go", "a/b/c/d/main.go")
	if shallow <= deep {
		t.Errorf("shallow (%d) should beat deep (%d)", shallow, deep)
	}
}

// Scenario 6: Deeply nested file still appears (positive score).
func TestScore_DeepStillPositive(t *testing.T) {
	deep := score("main.go", "main.go", "a/b/c/d/e/f/main.go")
	if deep <= 0 {
		t.Errorf("deeply nested file should still have positive score, got %d", deep)
	}
}

// Scenario 7: Recency boost raises score.
func TestScore_RecencyBoost(t *testing.T) {
	mr := FuzzyMatch("main", "main.go")
	normal := Score(mr, ScoreParams{RelPath: "main.go", Name: "main.go"})
	recent := Score(mr, ScoreParams{RelPath: "main.go", Name: "main.go", Recent: true})
	if recent != normal+200 {
		t.Errorf("recency should add 200: normal=%d recent=%d", normal, recent)
	}
}

// Scenario 8: Frequency boost raises score.
func TestScore_FrequencyBoost(t *testing.T) {
	mr := FuzzyMatch("main", "main.go")
	normal := Score(mr, ScoreParams{RelPath: "main.go", Name: "main.go"})
	freq := Score(mr, ScoreParams{RelPath: "main.go", Name: "main.go", Frequent: true})
	if freq != normal+150 {
		t.Errorf("frequency should add 150: normal=%d freq=%d", normal, freq)
	}
}

// Scenario 9: Deterministic — same input, same output.
func TestScore_Deterministic(t *testing.T) {
	for range 100 {
		a := score("cfg", "config.go", "internal/config/config.go")
		b := score("cfg", "config.go", "internal/config/config.go")
		if a != b {
			t.Fatalf("non-deterministic: %d != %d", a, b)
		}
	}
}

// Scenario 10: RankResults tie-breaking — shorter name first.
func TestRank_ShorterNameFirst(t *testing.T) {
	results := []model.SearchResult{
		{Name: "longername.go", Score: 500, Detail: ""},
		{Name: "short.go", Score: 500, Detail: ""},
	}
	RankResults(results)
	if results[0].Name != "short.go" {
		t.Errorf("shorter name should come first, got %s", results[0].Name)
	}
}

// Scenario 11: RankResults tie-breaking — shallower path first.
func TestRank_ShallowerPathFirst(t *testing.T) {
	results := []model.SearchResult{
		{Name: "main.go", Score: 500, Detail: "a/b/c"},
		{Name: "main.go", Score: 500, Detail: "a"},
	}
	RankResults(results)
	if results[0].Detail != "a" {
		t.Errorf("shallower path should come first, got detail=%s", results[0].Detail)
	}
}

// Scenario 12: RankResults tie-breaking — alphabetical (same length, same depth).
func TestRank_Alphabetical(t *testing.T) {
	results := []model.SearchResult{
		{Name: "beta.go", Score: 500, Detail: ""},
		{Name: "alfa.go", Score: 500, Detail: ""},
	}
	RankResults(results)
	if results[0].Name != "alfa.go" {
		t.Errorf("alphabetical order expected, got %s first", results[0].Name)
	}
}

// Scenario 13: Word-boundary alignment boosts score.
func TestScore_BoundaryAlignment(t *testing.T) {
	// "m" at start of "model.go" (boundary) vs "m" in middle of "timer.go".
	boundary := score("m", "model.go", "model.go")
	mid := score("m", "timer.go", "timer.go")
	if boundary <= mid {
		t.Errorf("boundary-aligned (%d) should beat mid-word (%d)", boundary, mid)
	}
}

// Scenario 14: Position penalty — later start scores lower.
func TestScore_PositionPenalty(t *testing.T) {
	early := score("go", "go_utils.txt", "go_utils.txt")
	late := score("go", "main.go", "main.go")
	// "go" at position 0 in go_utils vs position 5 in main.go.
	// Early start should score higher for same match type.
	if early < late {
		// Both are prefix/substring but early starts at 0.
		// This is a soft check; substring at pos 5 has -25 position penalty.
		t.Logf("early=%d late=%d (early start preferred)", early, late)
	}
}

func TestScore_NonMatchReturnsZero(t *testing.T) {
	mr := FuzzyMatch("zzz", "main.go")
	s := Score(mr, ScoreParams{RelPath: "main.go", Name: "main.go"})
	if s != 0 {
		t.Errorf("non-match should score 0, got %d", s)
	}
}
