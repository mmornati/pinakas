package actuals

import (
	"strings"
	"testing"
	"time"
)

func TestNewRoundsDuration(t *testing.T) {
	start := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	a := New("acme/app", 42, start, start.Add(25*time.Minute+40*time.Second))
	if a.DurationMin != 26 {
		t.Errorf("DurationMin = %d, want 26", a.DurationMin)
	}
	if neg := New("acme/app", 1, start, start.Add(-time.Minute)); neg.DurationMin != 0 {
		t.Errorf("negative duration should clamp to 0, got %d", neg.DurationMin)
	}
}

func TestParseFirstPass(t *testing.T) {
	if v, err := ParseFirstPass(""); v != nil || err != nil {
		t.Errorf("empty = %v, %v", v, err)
	}
	if v, _ := ParseFirstPass("Yes"); v == nil || !*v {
		t.Error("Yes should be true")
	}
	if v, _ := ParseFirstPass("no"); v == nil || *v {
		t.Error("no should be false")
	}
	if _, err := ParseFirstPass("maybe"); err == nil {
		t.Error("maybe should fail")
	}
}

func TestParseFindings(t *testing.T) {
	f, err := ParseFindings("Major=1, minor=3")
	if err != nil || f["major"] != 1 || f["minor"] != 3 {
		t.Fatalf("ParseFindings = %v, %v", f, err)
	}
	for _, bad := range []string{"major", "major=-1", "major=x", "=1"} {
		if _, err := ParseFindings(bad); err == nil {
			t.Errorf("ParseFindings(%q) should fail", bad)
		}
	}
}

func TestCommentRoundTrip(t *testing.T) {
	start := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	a := New("acme/app", 42, start, start.Add(26*time.Minute))
	tokens, pass := 120000, true
	a.Tokens, a.FirstPass = &tokens, &pass
	a.Findings = map[string]int{"minor": 3, "major": 1}
	a.Agent = "claude-code"

	body, err := a.Comment()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| Duration | 26 min |", "| Tokens | 120000 |", "| First pass | yes |", "| Findings | major=1, minor=3 |"} {
		if !strings.Contains(body, want) {
			t.Errorf("comment missing %q:\n%s", want, body)
		}
	}
	got, ok := Parse(body)
	if !ok || got.Number != 42 || *got.Tokens != tokens || got.Findings["major"] != 1 {
		t.Fatalf("Parse = %+v, %v", got, ok)
	}
	if _, ok := Parse("just a comment"); ok {
		t.Error("plain comment should not parse")
	}
}
