package pr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/board/boardtest"
	"github.com/mmornati/pinakas/internal/config"
)

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Board.Owner, cfg.Board.Project = "acme", 1
	return cfg
}

func newFake() *boardtest.Fake {
	return &boardtest.Fake{CardList: []board.Card{
		{ItemID: "I7", Repo: "acme/app", Number: 7, Title: "Login", Status: "In Progress"},
		{ItemID: "I8", Repo: "acme/app", Number: 8, Title: "Logout", Status: "Done"},
	}}
}

func event(action, body string) Event {
	var e Event
	e.Action = action
	e.Repository.FullName = "acme/app"
	e.PullRequest.Number = 21
	e.PullRequest.Body = body
	e.PullRequest.User.Login = "agent-bot"
	e.PullRequest.CreatedAt = time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	return e
}

func TestParseEvent(t *testing.T) {
	ev, err := ParseEvent(strings.NewReader(`{"action":"opened","pull_request":{"number":3,"body":"Closes #7","labels":[{"name":"Risk:High"}]},"repository":{"full_name":"acme/app"}}`))
	if err != nil || ev.PullRequest.Number != 3 || ev.Repository.FullName != "acme/app" {
		t.Fatalf("ParseEvent = %+v, %v", ev, err)
	}
	if l, ok := ev.PullRequest.HasLabel([]string{"risk:high"}); !ok || l != "Risk:High" {
		t.Errorf("HasLabel = %q, %v", l, ok)
	}
	if _, err := ParseEvent(strings.NewReader(`{"action":"push"}`)); err == nil {
		t.Error("an event without a pull request should fail")
	}
}

func TestLinkedIssues(t *testing.T) {
	body := "Closes #7, fixes: acme/api#3\nResolves https://github.com/acme/web/issues/12\ncloses #7\nSee #99, prefix#5, closesx #4"
	got := LinkedIssues(body, "acme/app")
	var refs []string
	for _, r := range got {
		refs = append(refs, r.String())
	}
	if want := "acme/app#7 acme/api#3 acme/web#12"; strings.Join(refs, " ") != want {
		t.Errorf("LinkedIssues = %v, want %s", refs, want)
	}
	if len(LinkedIssues("", "acme/app")) != 0 {
		t.Error("empty body links nothing")
	}
}

func TestSyncLifecycle(t *testing.T) {
	cfg, b, ctx := testConfig(), newFake(), context.Background()

	lines, err := Sync(ctx, cfg, b, event("opened", "Closes #7 and closes #8 and closes #40"))
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{`acme/app#7 moved from "In Progress" to "In Review"`, "acme/app#8 is done", "acme/app#40 is not on the board"} {
		if !strings.Contains(all, want) {
			t.Errorf("sync on open missing %q:\n%s", want, all)
		}
	}

	draft := event("converted_to_draft", "Closes #7")
	if _, err := Sync(ctx, cfg, b, draft); err != nil || b.CardList[0].Status != "In Progress" {
		t.Errorf("back to draft: status %q, %v", b.CardList[0].Status, err)
	}

	quiet := event("synchronize", "Closes #7")
	if lines, _ := Sync(ctx, cfg, b, quiet); !strings.Contains(lines[0], "Nothing to do") {
		t.Errorf("synchronize should do nothing, got %v", lines)
	}
	isDraft := event("opened", "Closes #7")
	isDraft.PullRequest.Draft = true
	if lines, _ := Sync(ctx, cfg, b, isDraft); !strings.Contains(lines[0], "draft") {
		t.Errorf("draft PR should not move, got %v", lines)
	}
	if lines, _ := Sync(ctx, cfg, b, event("opened", "no link")); !strings.Contains(lines[0], "closes no issue") {
		t.Errorf("no link: %v", lines)
	}
}

func TestSyncMergeRecordsTimingOnce(t *testing.T) {
	cfg, b, ctx := testConfig(), newFake(), context.Background()
	claimed := time.Date(2026, 11, 27, 9, 30, 0, 0, time.UTC)
	a := actuals.New("acme/app", 7, claimed, claimed.Add(20*time.Minute))
	body, _ := a.Comment()
	b.IssueNotes = map[string][]string{"acme/app#7": {body}}

	merged := event("closed", "Closes #7")
	merged.PullRequest.Merged = true
	at := time.Date(2026, 11, 27, 12, 15, 0, 0, time.UTC)
	merged.PullRequest.MergedAt = &at

	lines, err := Sync(ctx, cfg, b, merged)
	if err != nil {
		t.Fatal(err)
	}
	if b.CardList[0].Status != "Done" {
		t.Errorf("status = %q, want Done", b.CardList[0].Status)
	}
	if len(b.Posted) != 1 || !strings.Contains(b.Posted[0], "| Claim to merge | 2 h 45 min |") || !strings.Contains(b.Posted[0], "| Claim to pull request | 30 min |") {
		t.Fatalf("timing comment = %q", b.Posted)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "2 h 45 min from claim to merge") {
		t.Errorf("lines = %v", lines)
	}

	// A re-run of the same event must not post twice.
	if _, err := Sync(ctx, cfg, b, merged); err != nil || len(b.Posted) != 1 {
		t.Errorf("re-run posted %d comments, %v", len(b.Posted), err)
	}

	closed := event("closed", "Closes #7")
	if lines, _ := Sync(ctx, cfg, b, closed); !strings.Contains(lines[0], "without merge") {
		t.Errorf("closed without merge: %v", lines)
	}
}

func TestSyncMergeFallsBackToAssignment(t *testing.T) {
	cfg, b, ctx := testConfig(), newFake(), context.Background()
	b.Assigned1 = map[string]time.Time{"acme/app#7": time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)}
	merged := event("closed", "Closes #7")
	merged.PullRequest.Merged = true
	at := time.Date(2026, 11, 27, 11, 0, 0, 0, time.UTC)
	merged.PullRequest.MergedAt = &at
	if _, err := Sync(ctx, cfg, b, merged); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.Posted[0], "| Claim to merge | 3 h |") {
		t.Errorf("timing = %q", b.Posted[0])
	}

	b2 := newFake()
	if _, err := Sync(ctx, cfg, b2, merged); err != nil || !strings.Contains(b2.Posted[0], "Claim time unknown") {
		t.Errorf("unknown claim: %q, %v", b2.Posted, err)
	}
}

func TestCheck(t *testing.T) {
	cfg, ctx := testConfig(), context.Background()

	res, err := Check(ctx, cfg, newFake(), event("opened", "no link"))
	if err != nil || res.Pass || !strings.Contains(res.Lines[0], "closes no issue") {
		t.Errorf("no link: %+v, %v", res, err)
	}

	res, _ = Check(ctx, cfg, newFake(), event("opened", "Closes #40"))
	if res.Pass || !strings.Contains(strings.Join(res.Lines, "\n"), "not on the board") {
		t.Errorf("unknown issue: %+v", res)
	}

	res, _ = Check(ctx, cfg, newFake(), event("opened", "Closes #7"))
	if res.Pass || !strings.Contains(strings.Join(res.Lines, "\n"), "no actuals") {
		t.Errorf("missing actuals: %+v", res)
	}

	b := newFake()
	a := actuals.New("acme/app", 7, time.Now(), time.Now())
	body, _ := a.Comment()
	b.IssueNotes = map[string][]string{"acme/app#7": {body}}
	res, _ = Check(ctx, cfg, b, event("opened", "Closes #7"))
	if !res.Pass {
		t.Errorf("linked card with actuals should pass: %+v", res)
	}

	risky := event("opened", "Closes #7")
	risky.PullRequest.Labels = append(risky.PullRequest.Labels, struct {
		Name string `json:"name"`
	}{Name: "risk:high"})
	res, _ = Check(ctx, cfg, b, risky)
	if res.Pass || !strings.Contains(strings.Join(res.Lines, "\n"), "needs an approving review") {
		t.Errorf("risky without approval: %+v", res)
	}
	b.ReviewList = map[string][]board.Review{"acme/app#21": {
		{Login: "agent-bot", State: "APPROVED"},
		{Login: "copilot", Bot: true, State: "APPROVED"},
	}}
	if res, _ = Check(ctx, cfg, b, risky); res.Pass {
		t.Errorf("self and bot approvals must not count: %+v", res)
	}
	b.ReviewList["acme/app#21"] = append(b.ReviewList["acme/app#21"], board.Review{Login: "marco", State: "APPROVED"})
	if res, _ = Check(ctx, cfg, b, risky); !res.Pass || !strings.Contains(strings.Join(res.Lines, "\n"), "approved by marco") {
		t.Errorf("human approval should pass: %+v", res)
	}

	bot := event("opened", "bump deps")
	bot.PullRequest.User.Login = "dependabot[bot]"
	if res, _ = Check(ctx, cfg, newFake(), bot); !res.Pass {
		t.Errorf("exempt author should pass: %+v", res)
	}

	relaxed := testConfig()
	relaxed.Rules.RequireLinkedIssue, relaxed.Rules.RequireActualsOnPR = false, false
	if res, _ = Check(ctx, relaxed, newFake(), event("opened", "no link")); !res.Pass {
		t.Errorf("relaxed rules should pass: %+v", res)
	}
}

func TestHuman(t *testing.T) {
	for in, want := range map[int]string{0: "0 min", 59: "59 min", 60: "1 h", 125: "2 h 05 min"} {
		if got := human(in); got != want {
			t.Errorf("human(%d) = %q, want %q", in, got, want)
		}
	}
}
