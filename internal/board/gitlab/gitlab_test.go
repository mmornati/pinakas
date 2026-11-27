package gitlab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/board"
)

type fakeGitLab struct {
	t        *testing.T
	requests []string
	bodies   map[string]string
}

func issueJSON(id, iid int, title, state string, labels []string, assignees string) string {
	l, _ := json.Marshal(labels)
	return `{"id":` + itoa(id) + `,"iid":` + itoa(iid) + `,"title":"` + title + `","web_url":"https://gl/acme/app/-/issues/` + itoa(iid) +
		`","state":"` + state + `","labels":` + string(l) + `,"assignees":[` + assignees + `],"references":{"full":"acme/app#` + itoa(iid) + `"}}`
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func (f *fakeGitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("PRIVATE-TOKEN") != "tok" {
		http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v4/")
	key := r.Method + " " + path
	f.requests = append(f.requests, key+"?"+r.URL.RawQuery)
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		if len(raw) > 0 {
			f.bodies[key] = string(raw)
		}
	}
	q := r.URL.Query()
	switch {
	case key == "GET projects/acme%2Fapp/issues" && q.Get("labels") == "workflow::ready" && q.Get("page") == "1":
		w.Header().Set("X-Next-Page", "2")
		_, _ = io.WriteString(w, "["+issueJSON(101, 1, "Login", "opened", []string{"workflow::ready", "bug"}, "")+"]")
	case key == "GET projects/acme%2Fapp/issues" && q.Get("labels") == "workflow::ready" && q.Get("page") == "2":
		_, _ = io.WriteString(w, "["+issueJSON(102, 2, "Logout", "opened", []string{"workflow::ready"}, `{"id":7,"username":"someone"}`)+"]")
	case key == "GET projects/acme%2Fapp/issues" && q.Get("labels") == "workflow::done":
		_, _ = io.WriteString(w, "["+issueJSON(103, 3, "Profile", "closed", []string{"workflow::done"}, "")+"]")
	case key == "GET projects/acme%2Fapp/issues":
		_, _ = io.WriteString(w, "[]")
	case key == "GET user":
		_, _ = io.WriteString(w, `{"id":42,"username":"agent-bot"}`)
	case key == "GET users":
		if q.Get("username") == "ghost" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		id := "9"
		if q.Get("username") == "someone" {
			id = "7"
		}
		_, _ = io.WriteString(w, `[{"id":`+id+`,"username":"`+q.Get("username")+`"}]`)
	case key == "GET projects/acme%2Fapp/issues/2":
		_, _ = io.WriteString(w, issueJSON(102, 2, "Logout", "opened", nil, `{"id":7,"username":"someone"}`))
	case strings.HasPrefix(key, "PUT projects/acme%2Fapp/issues/"), strings.HasPrefix(key, "POST "):
		_, _ = io.WriteString(w, `{}`)
	case key == "GET projects/acme%2Fapp/issues/1/notes":
		_, _ = io.WriteString(w, `[
		 {"body":"assigned to @agent-bot","system":true,"created_at":"2026-11-20T10:00:00Z"},
		 {"body":"first","system":false,"created_at":"2026-11-20T10:05:00Z"},
		 {"body":"assigned to @other","system":true,"created_at":"2026-11-21T10:00:00Z"},
		 {"body":"second","system":false,"created_at":"2026-11-21T11:00:00Z"}]`)
	default:
		http.Error(w, `{"message":"404 Not found"}`, http.StatusNotFound)
	}
}

func newTestBoard(t *testing.T) (*Board, *fakeGitLab) {
	t.Helper()
	f := &fakeGitLab{t: t, bodies: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	b, err := New(Options{
		URL: srv.URL + "/", Token: "tok", Project: "acme/app",
		Columns: []string{"workflow::ready", "workflow::in progress", "workflow::review", "workflow::done"},
		Done:    "workflow::done", Client: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b, f
}

func TestCards(t *testing.T) {
	b, _ := newTestBoard(t)
	cards, err := b.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Fatalf("cards = %+v", cards)
	}
	want := board.Card{ItemID: "101", Repo: "acme/app", Number: 1, Title: "Login", URL: "https://gl/acme/app/-/issues/1", State: "OPEN", Status: "workflow::ready", Assignees: []string{}}
	if got := cards[0]; got.Ref() != want.Ref() || got.Status != want.Status || got.State != want.State || got.ItemID != want.ItemID || got.Assignees == nil {
		t.Errorf("card 1 = %+v", got)
	}
	if got := cards[1]; len(got.Assignees) != 1 || got.Assignees[0] != "someone" {
		t.Errorf("card 2 = %+v", got)
	}
	if got := cards[2]; got.State != "CLOSED" || got.Status != "workflow::done" {
		t.Errorf("card 3 = %+v", got)
	}
}

func TestMoveSwapsLabelsAndCloses(t *testing.T) {
	b, f := newTestBoard(t)
	ctx := context.Background()
	if err := b.Move(ctx, board.Card{Repo: "acme/app", Number: 1, State: "OPEN"}, "workflow::in progress"); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(f.bodies["PUT projects/acme%2Fapp/issues/1"]), &body)
	if body["add_labels"] != "workflow::in progress" || body["remove_labels"] != "workflow::ready,workflow::review,workflow::done" || body["state_event"] != nil {
		t.Errorf("move body = %v", body)
	}
	if err := b.Move(ctx, board.Card{Repo: "acme/app", Number: 1, State: "OPEN"}, "workflow::done"); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(f.bodies["PUT projects/acme%2Fapp/issues/1"]), &body)
	if body["state_event"] != "close" {
		t.Errorf("done must close: %v", body)
	}
	body = nil
	if err := b.Move(ctx, board.Card{Repo: "acme/app", Number: 3, State: "CLOSED"}, "workflow::ready"); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(f.bodies["PUT projects/acme%2Fapp/issues/3"]), &body)
	if body["state_event"] != "reopen" {
		t.Errorf("leaving done must reopen: %v", body)
	}
}

func TestAssignAndUnassign(t *testing.T) {
	b, f := newTestBoard(t)
	ctx := context.Background()
	me, err := b.CurrentUser(ctx)
	if err != nil || me != "agent-bot" {
		t.Fatalf("CurrentUser = %q, %v", me, err)
	}
	card := board.Card{Repo: "acme/app", Number: 2}
	if err := b.Assign(ctx, card, "agent-bot"); err != nil {
		t.Fatal(err)
	}
	if got := f.bodies["PUT projects/acme%2Fapp/issues/2"]; got != `{"assignee_ids":[7,42]}` {
		t.Errorf("assign body = %s", got)
	}
	if err := b.Unassign(ctx, card, "someone"); err != nil {
		t.Fatal(err)
	}
	if got := f.bodies["PUT projects/acme%2Fapp/issues/2"]; got != `{"assignee_ids":[0]}` {
		t.Errorf("unassign of the last assignee must clear: %s", got)
	}
	if err := b.Assign(ctx, card, "ghost"); err == nil || !strings.Contains(err.Error(), "no user") {
		t.Errorf("unknown user: %v", err)
	}
}

func TestNotes(t *testing.T) {
	b, f := newTestBoard(t)
	ctx := context.Background()
	comments, err := b.Comments(ctx, "acme/app", 1)
	if err != nil || strings.Join(comments, ",") != "first,second" {
		t.Errorf("Comments = %v, %v", comments, err)
	}
	at, err := b.FirstAssignedAt(ctx, "acme/app", 1)
	if err != nil || !at.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("FirstAssignedAt = %v, %v", at, err)
	}
	if err := b.Comment(ctx, board.Card{Repo: "acme/app", Number: 1}, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := f.bodies["POST projects/acme%2Fapp/issues/1/notes"]; got != `{"body":"hello"}` {
		t.Errorf("note body = %s", got)
	}
	if _, err := b.Reviews(ctx, "acme/app", 1); err == nil {
		t.Error("Reviews should be unsupported")
	}
}

func TestErrors(t *testing.T) {
	if _, err := New(Options{Project: "acme/app"}); err == nil {
		t.Error("missing token must fail")
	}
	if _, err := New(Options{Token: "x"}); err == nil {
		t.Error("missing project and group must fail")
	}
	b, _ := newTestBoard(t)
	b.token = "wrong"
	if _, err := b.CurrentUser(context.Background()); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("bad token: %v", err)
	}
	g, err := New(Options{Token: "x", Group: "acme/platform"})
	if err != nil || g.scope != "groups/acme%2Fplatform" {
		t.Errorf("group scope = %q, %v", g.scope, err)
	}
}
