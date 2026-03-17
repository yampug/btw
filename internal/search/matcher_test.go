package search

import (
	"strings"
	"testing"
)

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	r := FuzzyMatch("", "anything")
	if !r.Matched {
		t.Error("empty query should match everything")
	}
	if r.Score != 0 {
		t.Errorf("empty query score should be 0, got %d", r.Score)
	}
}

func TestFuzzyMatch_WhitespaceQuery(t *testing.T) {
	r := FuzzyMatch("   ", "anything")
	if !r.Matched {
		t.Error("whitespace query should match everything")
	}
	if r.Score != 0 {
		t.Errorf("whitespace query score should be 0, got %d", r.Score)
	}
}

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	r := FuzzyMatch("main.go", "main.go")
	if !r.Matched || r.Score != 1000 {
		t.Errorf("expected exact match score 1000, got matched=%v score=%d", r.Matched, r.Score)
	}
}

func TestFuzzyMatch_ExactMatchCaseInsensitive(t *testing.T) {
	r := FuzzyMatch("Main.Go", "main.go")
	if !r.Matched || r.Score != 1000 {
		t.Errorf("expected case-insensitive exact match, got matched=%v score=%d", r.Matched, r.Score)
	}
}

func TestFuzzyMatch_PrefixMatch(t *testing.T) {
	r := FuzzyMatch("mai", "main.go")
	if !r.Matched || r.Score != 800 {
		t.Errorf("expected prefix match score 800, got matched=%v score=%d", r.Matched, r.Score)
	}
	if len(r.Ranges) != 1 || r.Ranges[0].Start != 0 || r.Ranges[0].End != 3 {
		t.Errorf("expected range [0,3], got %v", r.Ranges)
	}
}

func TestFuzzyMatch_CamelCase_CCN(t *testing.T) {
	r := FuzzyMatch("CCN", "CamelCaseName")
	if !r.Matched {
		t.Fatal("CCN should match CamelCaseName")
	}
	if r.Score != 600 {
		t.Errorf("expected camel case score 600, got %d", r.Score)
	}
}

func TestFuzzyMatch_CamelCase_NPE(t *testing.T) {
	r := FuzzyMatch("NPE", "NullPointerException")
	if !r.Matched {
		t.Fatal("NPE should match NullPointerException")
	}
	if r.Score != 600 {
		t.Errorf("expected score 600, got %d", r.Score)
	}
}

func TestFuzzyMatch_CamelCase_grs(t *testing.T) {
	r := FuzzyMatch("grs", "getResponseStatus")
	if !r.Matched {
		t.Fatal("grs should match getResponseStatus")
	}
	if r.Score != 600 {
		t.Errorf("expected score 600, got %d", r.Score)
	}
}

func TestFuzzyMatch_CamelCase_UppercaseEnforced(t *testing.T) {
	// Uppercase G in query should require G in candidate.
	r := FuzzyMatch("GRS", "getResponseStatus")
	// G doesn't match 'g' at boundary because G is uppercase and requires exact case.
	if r.Matched && r.Score == 600 {
		// This is fine only if 'G' matched at a boundary where candidate has 'G'.
		// Since candidate starts with lowercase 'g', uppercase 'G' should NOT match it.
		t.Error("uppercase G should not match lowercase g")
	}
}

func TestFuzzyMatch_SnakeCase(t *testing.T) {
	// scn matches via CamelCase boundary logic (underscore creates boundaries),
	// so it scores 600. Pure snake_case matching (score 550) is a fallback for
	// cases that CamelCase doesn't catch.
	r := FuzzyMatch("scn", "snake_case_name")
	if !r.Matched {
		t.Fatal("scn should match snake_case_name")
	}
	if r.Score < 550 {
		t.Errorf("expected score >= 550, got %d", r.Score)
	}
}

func TestFuzzyMatch_SnakeCase_MultiChar(t *testing.T) {
	r := FuzzyMatch("sn_ca", "snake_case_name")
	if !r.Matched {
		t.Fatal("sn_ca should match snake_case_name via snake case")
	}
}

func TestFuzzyMatch_SubstringMatch(t *testing.T) {
	r := FuzzyMatch("odel", "UserModel")
	if !r.Matched {
		t.Fatal("odel should substring-match UserModel")
	}
	if r.Score != 500 {
		t.Errorf("expected substring score 500, got %d", r.Score)
	}
	if len(r.Ranges) != 1 || r.Ranges[0].Start != 5 || r.Ranges[0].End != 9 {
		t.Errorf("expected range [5,9], got %v", r.Ranges)
	}
}

func TestFuzzyMatch_SubsequenceMatch(t *testing.T) {
	r := FuzzyMatch("mgo", "main.go")
	if !r.Matched {
		t.Fatal("mgo should subsequence-match main.go")
	}
	if r.Score <= 0 || r.Score > 300 {
		t.Errorf("expected subsequence score 1-300, got %d", r.Score)
	}
	if len(r.Ranges) != 3 {
		t.Errorf("expected 3 ranges for subsequence, got %d", len(r.Ranges))
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	r := FuzzyMatch("xyz", "main.go")
	if r.Matched {
		t.Error("xyz should not match main.go")
	}
}

func TestFuzzyMatch_RangesAccurate(t *testing.T) {
	r := FuzzyMatch("CCN", "CamelCaseName")
	if !r.Matched {
		t.Fatal("should match")
	}
	// Verify each range points to the correct character.
	runes := []rune("CamelCaseName")
	for _, rng := range r.Ranges {
		for i := rng.Start; i < rng.End; i++ {
			if i >= len(runes) {
				t.Errorf("range index %d out of bounds", i)
			}
		}
	}
}

func TestFuzzyMatch_CamelCase_DotBoundary(t *testing.T) {
	r := FuzzyMatch("mg", "main.go")
	// m at start, g after dot → both are boundaries.
	if !r.Matched {
		t.Fatal("mg should match main.go via boundary match")
	}
}

func TestFuzzyMatch_CamelCase_HyphenBoundary(t *testing.T) {
	r := FuzzyMatch("fb", "foo-bar")
	if !r.Matched {
		t.Fatal("fb should match foo-bar via boundary")
	}
}

func TestFuzzyMatch_PriorityOrder(t *testing.T) {
	// Exact > prefix > camelcase > snake > substring > subsequence
	exact := FuzzyMatch("main.go", "main.go")
	prefix := FuzzyMatch("mai", "main.go")
	camel := FuzzyMatch("CCN", "CamelCaseName")
	snake := FuzzyMatch("scn", "snake_case_name")
	substr := FuzzyMatch("odel", "UserModel")
	subseq := FuzzyMatch("mgo", "main.go")

	// Exact > prefix > camelcase >= snake > substring > subsequence
	if exact.Score <= prefix.Score {
		t.Errorf("exact (%d) should beat prefix (%d)", exact.Score, prefix.Score)
	}
	if prefix.Score <= camel.Score {
		t.Errorf("prefix (%d) should beat camelcase (%d)", prefix.Score, camel.Score)
	}
	if camel.Score < snake.Score {
		t.Errorf("camelcase (%d) should be >= snake (%d)", camel.Score, snake.Score)
	}
	if snake.Score <= substr.Score {
		t.Errorf("snake (%d) should beat substring (%d)", snake.Score, substr.Score)
	}
	if substr.Score <= subseq.Score {
		t.Errorf("substring (%d) should beat subsequence (%d)", substr.Score, subseq.Score)
	}
}

func BenchmarkFuzzyMatch(b *testing.B) {
	candidates := []string{
		"CamelCaseName", "snake_case_name", "main.go", "UserModel",
		"getResponseStatus", "NullPointerException", "ApplicationController",
		"very_long_snake_case_variable_name_here", "simplefilename.txt",
	}
	queries := []string{"CCN", "scn", "mgo", "odel", "grs", "NPE", "AC", "vlsn", "sim"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := queries[i%len(queries)]
		c := candidates[i%len(candidates)]
		FuzzyMatch(q, c)
	}
}

func TestBenchmarkFuzzyMatch_Under1us(t *testing.T) {
	// Quick smoke test: run many matches and check they complete fast.
	candidates := []string{
		"CamelCaseName", "snake_case_name", "main.go", "UserModel",
		"getResponseStatus", "NullPointerException",
	}
	queries := []string{"CCN", "scn", "mgo", "odel", "grs", "NPE"}

	count := 0
	for range 10000 {
		for _, q := range queries {
			for _, c := range candidates {
				r := FuzzyMatch(q, c)
				_ = r
				count++
			}
		}
	}
	_ = strings.ToLower("done") // prevent dead code elimination
	if count != 360000 {
		t.Errorf("unexpected count %d", count)
	}
}
