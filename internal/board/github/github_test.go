package github

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/mmornati/pinakas/internal/board"
)

type fakeGQL struct {
	pages []string
	calls []map[string]any
	query []string
}

func (f *fakeGQL) DoWithContext(_ context.Context, q string, vars map[string]any, resp any) error {
	f.calls = append(f.calls, vars)
	f.query = append(f.query, q)
	if strings.HasPrefix(q, "mutation") {
		return nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return json.Unmarshal([]byte(page), resp)
}

type fakeREST struct {
	method, path, body string
}

func (f *fakeREST) DoWithContext(_ context.Context, method, path string, body io.Reader, resp any) error {
	f.method, f.path = method, path
	if body != nil {
		raw, _ := io.ReadAll(body)
		f.body = string(raw)
	}
	if path == "user" {
		return json.Unmarshal([]byte(`{"login":"agent-bot"}`), resp)
	}
	return nil
}

const page1 = `{"repositoryOwner":{"projectV2":{"id":"PVT_1",
 "field":{"id":"F_1","options":[{"id":"o1","name":"Ready"},{"id":"o2","name":"In Progress"}]},
 "items":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},"nodes":[
  {"id":"I_1","fieldValueByName":{"name":"Ready"},"content":{"number":7,"title":"Login","url":"u7","state":"OPEN","repository":{"nameWithOwner":"acme/app"},"assignees":{"nodes":[]}}},
  {"id":"I_draft","fieldValueByName":null,"content":{}}
 ]}}}}`

const page2 = `{"repositoryOwner":{"projectV2":{"id":"PVT_1",
 "field":{"id":"F_1","options":[{"id":"o1","name":"Ready"},{"id":"o2","name":"In Progress"}]},
 "items":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
  {"id":"I_2","fieldValueByName":{"name":"In Progress"},"content":{"number":8,"title":"Logout","url":"u8","state":"OPEN","repository":{"nameWithOwner":"acme/app"},"assignees":{"nodes":[{"login":"agent-bot"}]}}}
 ]}}}}`

func TestCardsPaginatesAndSkipsDrafts(t *testing.T) {
	gql := &fakeGQL{pages: []string{page1, page2}}
	b := NewWithClients(gql, &fakeREST{}, "acme", 3, "Status")

	cards, err := b.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("got %d cards, want 2: %+v", len(cards), cards)
	}
	if cards[1].Status != "In Progress" || cards[1].Assignees[0] != "agent-bot" {
		t.Errorf("card 2 = %+v", cards[1])
	}
	if gql.calls[1]["after"] == nil || *(gql.calls[1]["after"].(*string)) != "c1" {
		t.Errorf("second page should use the cursor, vars = %v", gql.calls[1])
	}
	// Cached: no further query.
	if _, err := b.Cards(context.Background()); err != nil || len(gql.calls) != 2 {
		t.Errorf("Cards should be cached, calls = %d", len(gql.calls))
	}
}

func TestMove(t *testing.T) {
	gql := &fakeGQL{pages: []string{page2}}
	b := NewWithClients(gql, &fakeREST{}, "acme", 3, "Status")
	card := board.Card{ItemID: "I_1", Repo: "acme/app", Number: 7}

	if err := b.Move(context.Background(), card, "In Progress"); err != nil {
		t.Fatal(err)
	}
	last := gql.calls[len(gql.calls)-1]
	if last["option"] != "o2" || last["field"] != "F_1" || last["project"] != "PVT_1" || last["item"] != "I_1" {
		t.Errorf("mutation vars = %v", last)
	}
	if err := b.Move(context.Background(), card, "Blocked"); err == nil || !strings.Contains(err.Error(), "Blocked") {
		t.Errorf("unknown column should fail, got %v", err)
	}
}

func TestMissingProjectAndField(t *testing.T) {
	b := NewWithClients(&fakeGQL{pages: []string{`{"repositoryOwner":{}}`}}, &fakeREST{}, "acme", 3, "Status")
	if _, err := b.Cards(context.Background()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing project err = %v", err)
	}
	b = NewWithClients(&fakeGQL{pages: []string{`{"repositoryOwner":{"projectV2":{"id":"P","field":null,"items":{"nodes":[]}}}}`}}, &fakeREST{}, "acme", 3, "Status")
	if _, err := b.Cards(context.Background()); err == nil || !strings.Contains(err.Error(), "status_field") {
		t.Errorf("missing field err = %v", err)
	}
}

func TestRESTCalls(t *testing.T) {
	rest := &fakeREST{}
	b := NewWithClients(&fakeGQL{}, rest, "acme", 3, "Status")
	card := board.Card{Repo: "acme/app", Number: 7}
	ctx := context.Background()

	login, err := b.CurrentUser(ctx)
	if err != nil || login != "agent-bot" {
		t.Fatalf("CurrentUser = %q, %v", login, err)
	}
	if err := b.Assign(ctx, card, "agent-bot"); err != nil || rest.path != "repos/acme/app/issues/7/assignees" || !strings.Contains(rest.body, "agent-bot") {
		t.Errorf("Assign: %v, %s %s", err, rest.path, rest.body)
	}
	if err := b.Comment(ctx, card, "hi"); err != nil || rest.method != "POST" || rest.path != "repos/acme/app/issues/7/comments" || rest.body != `{"body":"hi"}` {
		t.Errorf("Comment: %v, %s %s %s", err, rest.method, rest.path, rest.body)
	}
}

type scriptedREST struct {
	responses map[string]string
}

func (s *scriptedREST) DoWithContext(_ context.Context, _ string, path string, _ io.Reader, resp any) error {
	body, ok := s.responses[path]
	if !ok {
		body = "[]"
	}
	return json.Unmarshal([]byte(body), resp)
}

func TestCommentsEventsReviews(t *testing.T) {
	rest := &scriptedREST{responses: map[string]string{
		"repos/acme/app/issues/7/comments?per_page=100&page=1": `[{"body":"a"},{"body":"b"}]`,
		"repos/acme/app/issues/7/events?per_page=100&page=1":   `[{"event":"labeled","created_at":"2026-11-27T08:00:00Z"},{"event":"assigned","created_at":"2026-11-27T09:00:00Z"},{"event":"assigned","created_at":"2026-11-27T10:00:00Z"}]`,
		"repos/acme/app/pulls/9/reviews?per_page=100&page=1": `[
		  {"state":"CHANGES_REQUESTED","user":{"login":"marco","type":"User"}},
		  {"state":"APPROVED","user":{"login":"marco","type":"User"}},
		  {"state":"COMMENTED","user":{"login":"marco","type":"User"}},
		  {"state":"COMMENTED","user":{"login":"copilot","type":"Bot"}}]`,
	}}
	b := NewWithClients(&fakeGQL{}, rest, "acme", 3, "Status")
	ctx := context.Background()

	comments, err := b.Comments(ctx, "acme/app", 7)
	if err != nil || strings.Join(comments, ",") != "a,b" {
		t.Errorf("Comments = %v, %v", comments, err)
	}
	at, err := b.FirstAssignedAt(ctx, "acme/app", 7)
	if err != nil || at.Hour() != 9 {
		t.Errorf("FirstAssignedAt = %v, %v", at, err)
	}
	if at, _ := b.FirstAssignedAt(ctx, "acme/app", 8); !at.IsZero() {
		t.Errorf("never assigned should be zero, got %v", at)
	}
	reviews, err := b.Reviews(ctx, "acme/app", 9)
	if err != nil || len(reviews) != 2 {
		t.Fatalf("Reviews = %+v, %v", reviews, err)
	}
	if reviews[0].State != "APPROVED" || reviews[0].Bot {
		t.Errorf("marco's latest state should stay APPROVED, got %+v", reviews[0])
	}
	if !reviews[1].Bot {
		t.Errorf("copilot should be a bot, got %+v", reviews[1])
	}
}
