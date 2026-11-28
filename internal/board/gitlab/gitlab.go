// Package gitlab implements the board on GitLab issues. A column is a
// label (scoped labels such as workflow::ready work well), as on a GitLab
// issue board: moving a card swaps its column label.
package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmornati/pinakas/internal/board"
)

// DefaultURL is used when board.url is not set.
const DefaultURL = "https://gitlab.com"

// Board reads and writes cards through the GitLab REST API (v4).
type Board struct {
	base    string
	token   string
	client  *http.Client
	scope   string // "projects/<id>" or "groups/<id>"
	columns []string
	done    string

	userIDs map[string]int
}

// Options configures a Board.
type Options struct {
	// URL of the GitLab instance; DefaultURL when empty.
	URL string
	// Token is a personal, project or group access token with the api scope.
	Token string
	// Group, when set, makes the board span every project of the group.
	Group string
	// Project is the project path (group/name) used when Group is empty.
	Project string
	// Columns are the column labels in workflow order.
	Columns []string
	// Done is the column given to closed issues that carry no column label.
	Done string
	// Client is the HTTP client; http.DefaultClient when nil.
	Client *http.Client
}

// TokenFromEnv returns GITLAB_TOKEN, or GL_TOKEN as glab does.
func TokenFromEnv() string {
	if t := os.Getenv("GITLAB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GL_TOKEN")
}

// New returns a board for the options.
func New(o Options) (*Board, error) {
	if o.Token == "" {
		return nil, errors.New("gitlab: no token (set GITLAB_TOKEN to a token with the api scope)")
	}
	base := strings.TrimRight(o.URL, "/")
	if base == "" {
		base = DefaultURL
	}
	scope := "projects/" + url.PathEscape(o.Project)
	if o.Group != "" {
		scope = "groups/" + url.PathEscape(o.Group)
	} else if o.Project == "" {
		return nil, errors.New("gitlab: set board.repo (project path) or board.owner (group path)")
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Board{
		base: base + "/api/v4/", token: o.Token, client: client, scope: scope,
		columns: o.Columns, done: o.Done, userIDs: map[string]int{},
	}, nil
}

type user struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type issue struct {
	ID         int      `json:"id"`
	IID        int      `json:"iid"`
	Title      string   `json:"title"`
	WebURL     string   `json:"web_url"`
	State      string   `json:"state"`
	Labels     []string `json:"labels"`
	Assignees  []user   `json:"assignees"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
}

// Cards lists the issues that carry a column label, and closed issues as done.
func (b *Board) Cards(ctx context.Context) ([]board.Card, error) {
	seen := map[int]bool{}
	var cards []board.Card
	add := func(issues []issue) {
		for _, is := range issues {
			if seen[is.ID] {
				continue
			}
			seen[is.ID] = true
			cards = append(cards, b.card(is))
		}
	}
	for _, label := range b.columns {
		q := url.Values{"labels": {label}, "state": {"all"}, "scope": {"all"}}
		issues, err := b.issues(ctx, q)
		if err != nil {
			return nil, err
		}
		add(issues)
	}
	return cards, nil
}

func (b *Board) issues(ctx context.Context, q url.Values) ([]issue, error) {
	var all []issue
	q.Set("per_page", "100")
	for page := 1; ; page++ {
		q.Set("page", strconv.Itoa(page))
		var batch []issue
		next, err := b.do(ctx, http.MethodGet, b.scope+"/issues?"+q.Encode(), nil, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if next == "" || len(batch) == 0 {
			return all, nil
		}
	}
}

func (b *Board) card(is issue) board.Card {
	repo, _, _ := strings.Cut(is.References.Full, "#")
	c := board.Card{
		ItemID: strconv.Itoa(is.ID), Repo: repo, Number: is.IID, Title: is.Title, URL: is.WebURL,
		State: "OPEN", Assignees: []string{},
	}
	if is.State == "closed" {
		c.State = "CLOSED"
	}
	for _, label := range b.columns {
		if slices.Contains(is.Labels, label) {
			c.Status = label
			break
		}
	}
	if c.Status == "" && is.State == "closed" {
		c.Status = b.done
	}
	for _, a := range is.Assignees {
		c.Assignees = append(c.Assignees, a.Username)
		b.userIDs[a.Username] = a.ID
	}
	return c
}

func issuePath(repo string, number int) string {
	return fmt.Sprintf("projects/%s/issues/%d", url.PathEscape(repo), number)
}

// Move swaps the card's column label. Moving to the done column also
// closes the issue, as a GitLab board does; moving out of it reopens it.
func (b *Board) Move(ctx context.Context, card board.Card, status string) error {
	var remove []string
	for _, l := range b.columns {
		if l != status {
			remove = append(remove, l)
		}
	}
	payload := map[string]any{"add_labels": status, "remove_labels": strings.Join(remove, ",")}
	if status == b.done {
		payload["state_event"] = "close"
	} else if card.State == "CLOSED" {
		payload["state_event"] = "reopen"
	}
	_, err := b.do(ctx, http.MethodPut, issuePath(card.Repo, card.Number), payload, nil)
	return err
}

func (b *Board) userID(ctx context.Context, login string) (int, error) {
	if id, ok := b.userIDs[login]; ok {
		return id, nil
	}
	var users []user
	if _, err := b.do(ctx, http.MethodGet, "users?username="+url.QueryEscape(login), nil, &users); err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, fmt.Errorf("gitlab: no user %q", login)
	}
	b.userIDs[login] = users[0].ID
	return users[0].ID, nil
}

func (b *Board) setAssignees(ctx context.Context, card board.Card, change func([]int) []int) error {
	var is issue
	if _, err := b.do(ctx, http.MethodGet, issuePath(card.Repo, card.Number), nil, &is); err != nil {
		return err
	}
	ids := make([]int, 0, len(is.Assignees)+1)
	for _, a := range is.Assignees {
		ids = append(ids, a.ID)
	}
	ids = change(ids)
	if len(ids) == 0 {
		ids = []int{0} // GitLab clears the assignees with 0
	}
	_, err := b.do(ctx, http.MethodPut, issuePath(card.Repo, card.Number), map[string]any{"assignee_ids": ids}, nil)
	return err
}

// Assign adds the user to the issue's assignees.
func (b *Board) Assign(ctx context.Context, card board.Card, login string) error {
	id, err := b.userID(ctx, login)
	if err != nil {
		return err
	}
	return b.setAssignees(ctx, card, func(ids []int) []int {
		if slices.Contains(ids, id) {
			return ids
		}
		return append(ids, id)
	})
}

// Unassign removes the user from the issue's assignees.
func (b *Board) Unassign(ctx context.Context, card board.Card, login string) error {
	id, err := b.userID(ctx, login)
	if err != nil {
		return err
	}
	return b.setAssignees(ctx, card, func(ids []int) []int {
		return slices.DeleteFunc(ids, func(v int) bool { return v == id })
	})
}

// Comment adds a note to the issue.
func (b *Board) Comment(ctx context.Context, card board.Card, body string) error {
	_, err := b.do(ctx, http.MethodPost, issuePath(card.Repo, card.Number)+"/notes", map[string]string{"body": body}, nil)
	return err
}

// CurrentUser returns the username of the token.
func (b *Board) CurrentUser(ctx context.Context) (string, error) {
	var u user
	if _, err := b.do(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	b.userIDs[u.Username] = u.ID
	return u.Username, nil
}

type note struct {
	Body      string    `json:"body"`
	System    bool      `json:"system"`
	CreatedAt time.Time `json:"created_at"`
}

func (b *Board) notes(ctx context.Context, repo string, number int) ([]note, error) {
	var all []note
	for page := 1; ; page++ {
		var batch []note
		path := fmt.Sprintf("%s/notes?sort=asc&order_by=created_at&per_page=100&page=%d", issuePath(repo, number), page)
		next, err := b.do(ctx, http.MethodGet, path, nil, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if next == "" || len(batch) == 0 {
			return all, nil
		}
	}
}

// Comments returns the bodies of the issue's notes, oldest first, without
// system notes.
func (b *Board) Comments(ctx context.Context, repo string, number int) ([]string, error) {
	notes, err := b.notes(ctx, repo, number)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range notes {
		if !n.System {
			out = append(out, n.Body)
		}
	}
	return out, nil
}

// FirstAssignedAt reads the first "assigned to @user" system note.
func (b *Board) FirstAssignedAt(ctx context.Context, repo string, number int) (time.Time, error) {
	notes, err := b.notes(ctx, repo, number)
	if err != nil {
		return time.Time{}, err
	}
	for _, n := range notes {
		if n.System && strings.HasPrefix(n.Body, "assigned to @") {
			return n.CreatedAt, nil
		}
	}
	return time.Time{}, nil
}

// ErrUnsupported is returned for features that only exist on GitHub so far.
var ErrUnsupported = errors.New("not supported on GitLab yet")

// Reviews is not available on GitLab yet: the required check (check-ci)
// reads GitHub pull request events.
func (b *Board) Reviews(context.Context, string, int) ([]board.Review, error) {
	return nil, fmt.Errorf("merge request approvals: %w", ErrUnsupported)
}

// do sends one request. It returns the X-Next-Page header for pagination.
func (b *Board) do(ctx context.Context, method, path string, payload, out any) (string, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		body = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", b.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("gitlab: %s %s: %s: %s", method, strings.SplitN(path, "?", 2)[0], resp.Status, msg)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return "", fmt.Errorf("gitlab: decode %s: %w", path, err)
		}
	}
	return resp.Header.Get("X-Next-Page"), nil
}
