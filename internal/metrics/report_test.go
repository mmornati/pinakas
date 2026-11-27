package metrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/board/boardtest"
	"github.com/mmornati/pinakas/internal/pr"
)

var day = 24 * time.Hour

func actualsNote(t *testing.T, number int, claimed time.Time, d time.Duration, tokens int, pass bool, findings map[string]int) string {
	t.Helper()
	a := actuals.New("acme/app", number, claimed, claimed.Add(d))
	a.Tokens, a.FirstPass, a.Findings = &tokens, &pass, findings
	body, err := a.Comment()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func timingNote(t *testing.T, number int, claimed, opened, merged time.Time) string {
	t.Helper()
	body, err := pr.NewTiming(pr.IssueRef{Repo: "acme/app", Number: number}, 100+number, claimed, opened, merged).Comment()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func defectNote(t *testing.T, story, bug int, found time.Time) string {
	t.Helper()
	body, err := Defect{Version: 1, Repo: "acme/app", Number: story, DefectRepo: "acme/app", DefectNumber: bug, FoundAt: found}.Comment()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestDefectRoundTrip(t *testing.T) {
	found := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	d := Defect{Version: 1, Repo: "acme/app", Number: 3, DefectRepo: "acme/api", DefectNumber: 9, FoundAt: found, Note: "null session"}
	body, err := d.Comment()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "acme/api#9 was found on 2026-11-20") || !strings.Contains(body, "null session") {
		t.Errorf("comment = %q", body)
	}
	got, ok := ParseDefect(body)
	if !ok || got != d {
		t.Errorf("ParseDefect = %+v, %v", got, ok)
	}
	if _, ok := ParseDefect("no marker"); ok {
		t.Error("parsed a comment without marker")
	}
	if _, ok := ParseDefect(DefectMarker + "{bad -->"); ok {
		t.Error("parsed bad JSON")
	}
}

func TestCollectAndSummarize(t *testing.T) {
	now := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	old := now.Add(-60 * day)
	recent := now.Add(-5 * day)
	f := &boardtest.Fake{
		CardList: []board.Card{
			{Repo: "acme/app", Number: 1, Title: "Old with defect", Status: "Done"},
			{Repo: "acme/app", Number: 2, Title: "Old clean", Status: "Done"},
			{Repo: "acme/app", Number: 3, Title: "Recent", Status: "Done"},
			{Repo: "acme/app", Number: 4, Title: "Unmeasured", Status: "Done"},
			{Repo: "acme/app", Number: 5, Title: "Ready", Status: "Ready"},
		},
		IssueNotes: map[string][]string{
			"acme/app#1": {
				actualsNote(t, 1, old, 40*time.Minute, 1000, false, map[string]int{"major": 1}),
				timingNote(t, 1, old, old.Add(40*time.Minute), old.Add(2*time.Hour)),
				defectNote(t, 1, 10, old.Add(10*day)),
				defectNote(t, 1, 11, old.Add(45*day)), // outside the window
			},
			"acme/app#2": {
				actualsNote(t, 2, old.Add(day), 20*time.Minute, 500, true, map[string]int{"minor": 2}),
				"an unrelated comment",
			},
			"acme/app#3": {
				actualsNote(t, 3, recent, 10*time.Minute, 300, true, nil),
				timingNote(t, 3, recent, recent.Add(10*time.Minute), recent.Add(70*time.Minute)),
			},
		},
	}
	ctx := context.Background()

	stories, err := Collect(ctx, f, Filter{Statuses: []string{"Done"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	var refs []string
	for _, s := range stories {
		refs = append(refs, s.Ref())
	}
	if got := strings.Join(refs, " "); got != "acme/app#1 acme/app#2 acme/app#3 acme/app#4" {
		t.Errorf("order = %q", got)
	}
	if s := stories[0]; s.Defects30 != 1 || len(s.Defects) != 2 || s.WindowOpen || !s.DeliveredAt.Equal(old.Add(2*time.Hour)) {
		t.Errorf("story 1 = %+v", s)
	}
	if s := stories[1]; s.DeliveredAt == nil || !s.DeliveredAt.Equal(old.Add(day+20*time.Minute)) {
		t.Errorf("story 2 delivered at actuals time, got %v", s.DeliveredAt)
	}
	if !stories[2].WindowOpen {
		t.Error("story 3 window should still be open")
	}

	sum := Summarize(stories)
	if sum.Stories != 4 || sum.WithActuals != 3 || sum.TotalTokens != 1800 || sum.TotalDuration != 70 {
		t.Errorf("summary = %+v", sum)
	}
	if sum.MedianDuration == nil || *sum.MedianDuration != 20 {
		t.Errorf("median duration = %v", sum.MedianDuration)
	}
	if sum.FirstPassRate == nil || *sum.FirstPassRate < 0.66 || *sum.FirstPassRate > 0.67 {
		t.Errorf("first pass = %v", sum.FirstPassRate)
	}
	if sum.MedianLeadTime == nil || *sum.MedianLeadTime != 95 {
		t.Errorf("median lead = %v", sum.MedianLeadTime)
	}
	if sum.StoriesClosed30 != 2 || sum.DefectRate30 == nil || *sum.DefectRate30 != 0.5 || sum.Defects30 != 1 {
		t.Errorf("defects = %d closed, rate %v", sum.StoriesClosed30, sum.DefectRate30)
	}
	if got := FormatFindings(sum.Findings); got != "major=1, minor=2" {
		t.Errorf("findings = %q", got)
	}

	measured, _ := Collect(ctx, f, Filter{Measured: true, Since: now.Add(-30 * day)}, now)
	if len(measured) != 1 || measured[0].Number != 3 {
		t.Errorf("measured since = %+v", measured)
	}
	all, _ := Collect(ctx, f, Filter{}, now)
	if len(all) != 5 {
		t.Errorf("all = %d stories", len(all))
	}
	if empty := Summarize(nil); empty.MedianDuration != nil || empty.FirstPassRate != nil || empty.DefectRate30 != nil {
		t.Errorf("empty summary = %+v", empty)
	}
}
