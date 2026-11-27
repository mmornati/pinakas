// Package github implements the board on GitHub Projects (v2).
//
// Authentication reuses the GitHub CLI: GH_TOKEN, GITHUB_TOKEN or the
// token stored by `gh auth login`.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/mmornati/pinakas/internal/board"
)

// GraphQL is the subset of the go-gh GraphQL client used here.
type GraphQL interface {
	DoWithContext(ctx context.Context, query string, variables map[string]any, response any) error
}

// REST is the subset of the go-gh REST client used here.
type REST interface {
	DoWithContext(ctx context.Context, method, path string, body io.Reader, response any) error
}

// Board is a GitHub Project (v2) used as a Kanban board.
type Board struct {
	gql         GraphQL
	rest        REST
	owner       string
	number      int
	statusField string

	once      sync.Once
	loadErr   error
	projectID string
	fieldID   string
	options   map[string]string // option name -> option id
	cards     []board.Card
}

// New returns a board backed by the default gh credentials.
func New(owner string, project int, statusField string) (*Board, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("github auth: %w (run `gh auth login` or set GH_TOKEN)", err)
	}
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("github auth: %w", err)
	}
	return NewWithClients(gql, rest, owner, project, statusField), nil
}

// NewWithClients builds a board from explicit clients (used in tests).
func NewWithClients(gql GraphQL, rest REST, owner string, project int, statusField string) *Board {
	return &Board{gql: gql, rest: rest, owner: owner, number: project, statusField: statusField}
}

const projectQuery = `query($owner: String!, $number: Int!, $field: String!, $after: String) {
  repositoryOwner(login: $owner) {
    ... on ProjectV2Owner {
      projectV2(number: $number) {
        id
        field(name: $field) {
          ... on ProjectV2SingleSelectField { id options { id name } }
        }
        items(first: 100, after: $after) {
          pageInfo { hasNextPage endCursor }
          nodes {
            id
            fieldValueByName(name: $field) {
              ... on ProjectV2ItemFieldSingleSelectValue { name }
            }
            content {
              ... on Issue {
                number title url state
                repository { nameWithOwner }
                assignees(first: 20) { nodes { login } }
              }
            }
          }
        }
      }
    }
  }
}`

type projectResponse struct {
	RepositoryOwner *struct {
		ProjectV2 *struct {
			ID    string `json:"id"`
			Field *struct {
				ID      string `json:"id"`
				Options []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"options"`
			} `json:"field"`
			Items struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []struct {
					ID               string `json:"id"`
					FieldValueByName *struct {
						Name string `json:"name"`
					} `json:"fieldValueByName"`
					Content *struct {
						Number     int    `json:"number"`
						Title      string `json:"title"`
						URL        string `json:"url"`
						State      string `json:"state"`
						Repository struct {
							NameWithOwner string `json:"nameWithOwner"`
						} `json:"repository"`
						Assignees struct {
							Nodes []struct {
								Login string `json:"login"`
							} `json:"nodes"`
						} `json:"assignees"`
					} `json:"content"`
				} `json:"nodes"`
			} `json:"items"`
		} `json:"projectV2"`
	} `json:"repositoryOwner"`
}

func (b *Board) load(ctx context.Context) error {
	b.once.Do(func() { b.loadErr = b.fetch(ctx) })
	return b.loadErr
}

func (b *Board) fetch(ctx context.Context) error {
	var after *string
	for {
		var resp projectResponse
		vars := map[string]any{"owner": b.owner, "number": b.number, "field": b.statusField, "after": after}
		if err := b.gql.DoWithContext(ctx, projectQuery, vars, &resp); err != nil {
			return fmt.Errorf("load project %s/%d: %w", b.owner, b.number, err)
		}
		if resp.RepositoryOwner == nil || resp.RepositoryOwner.ProjectV2 == nil {
			return fmt.Errorf("project %d not found for %q (check board.owner and board.project, and that the token has the project scope)", b.number, b.owner)
		}
		p := resp.RepositoryOwner.ProjectV2
		if p.Field == nil || p.Field.ID == "" {
			return fmt.Errorf("project %d has no single-select field named %q (set board.status_field)", b.number, b.statusField)
		}
		if b.projectID == "" {
			b.projectID, b.fieldID = p.ID, p.Field.ID
			b.options = make(map[string]string, len(p.Field.Options))
			for _, o := range p.Field.Options {
				b.options[o.Name] = o.ID
			}
		}
		for _, n := range p.Items.Nodes {
			if n.Content == nil || n.Content.Number == 0 {
				continue // draft issue or pull request
			}
			c := board.Card{
				ItemID: n.ID,
				Repo:   n.Content.Repository.NameWithOwner,
				Number: n.Content.Number,
				Title:  n.Content.Title,
				URL:    n.Content.URL,
				State:  n.Content.State,
			}
			if n.FieldValueByName != nil {
				c.Status = n.FieldValueByName.Name
			}
			for _, a := range n.Content.Assignees.Nodes {
				c.Assignees = append(c.Assignees, a.Login)
			}
			b.cards = append(b.cards, c)
		}
		if !p.Items.PageInfo.HasNextPage {
			return nil
		}
		cursor := p.Items.PageInfo.EndCursor
		after = &cursor
	}
}

// Cards implements board.Board.
func (b *Board) Cards(ctx context.Context) ([]board.Card, error) {
	if err := b.load(ctx); err != nil {
		return nil, err
	}
	return b.cards, nil
}

const moveMutation = `mutation($project: ID!, $item: ID!, $field: ID!, $option: String!) {
  updateProjectV2ItemFieldValue(input: {projectId: $project, itemId: $item, fieldId: $field, value: {singleSelectOptionId: $option}}) {
    projectV2Item { id }
  }
}`

// Move implements board.Board.
func (b *Board) Move(ctx context.Context, card board.Card, status string) error {
	if err := b.load(ctx); err != nil {
		return err
	}
	optionID, ok := b.options[status]
	if !ok {
		names := make([]string, 0, len(b.options))
		for n := range b.options {
			names = append(names, n)
		}
		return fmt.Errorf("column %q does not exist on the board (available: %s)", status, strings.Join(names, ", "))
	}
	vars := map[string]any{"project": b.projectID, "item": card.ItemID, "field": b.fieldID, "option": optionID}
	if err := b.gql.DoWithContext(ctx, moveMutation, vars, &struct{}{}); err != nil {
		return fmt.Errorf("move %s to %q: %w", card.Ref(), status, err)
	}
	return nil
}

// Assign implements board.Board.
func (b *Board) Assign(ctx context.Context, card board.Card, login string) error {
	path := fmt.Sprintf("repos/%s/issues/%d/assignees", card.Repo, card.Number)
	if err := b.post(ctx, path, map[string]any{"assignees": []string{login}}); err != nil {
		return fmt.Errorf("assign %s to %s: %w", card.Ref(), login, err)
	}
	return nil
}

// Comment implements board.Board.
func (b *Board) Comment(ctx context.Context, card board.Card, body string) error {
	path := fmt.Sprintf("repos/%s/issues/%d/comments", card.Repo, card.Number)
	if err := b.post(ctx, path, map[string]any{"body": body}); err != nil {
		return fmt.Errorf("comment on %s: %w", card.Ref(), err)
	}
	return nil
}

// CurrentUser implements board.Board.
func (b *Board) CurrentUser(ctx context.Context) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := b.rest.DoWithContext(ctx, "GET", "user", nil, &u); err != nil {
		return "", fmt.Errorf("read current user: %w", err)
	}
	return u.Login, nil
}

func (b *Board) post(ctx context.Context, path string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return b.rest.DoWithContext(ctx, "POST", path, bytes.NewReader(raw), nil)
}

// Comments implements board.Board.
func (b *Board) Comments(ctx context.Context, repo string, number int) ([]string, error) {
	var bodies []string
	for page := 1; ; page++ {
		var comments []struct {
			Body string `json:"body"`
		}
		path := fmt.Sprintf("repos/%s/issues/%d/comments?per_page=100&page=%d", repo, number, page)
		if err := b.rest.DoWithContext(ctx, "GET", path, nil, &comments); err != nil {
			return nil, fmt.Errorf("read comments of %s#%d: %w", repo, number, err)
		}
		for _, c := range comments {
			bodies = append(bodies, c.Body)
		}
		if len(comments) < 100 {
			return bodies, nil
		}
	}
}

// FirstAssignedAt implements board.Board.
func (b *Board) FirstAssignedAt(ctx context.Context, repo string, number int) (time.Time, error) {
	for page := 1; ; page++ {
		var events []struct {
			Event     string    `json:"event"`
			CreatedAt time.Time `json:"created_at"`
		}
		path := fmt.Sprintf("repos/%s/issues/%d/events?per_page=100&page=%d", repo, number, page)
		if err := b.rest.DoWithContext(ctx, "GET", path, nil, &events); err != nil {
			return time.Time{}, fmt.Errorf("read events of %s#%d: %w", repo, number, err)
		}
		for _, e := range events {
			if e.Event == "assigned" {
				return e.CreatedAt, nil
			}
		}
		if len(events) < 100 {
			return time.Time{}, nil
		}
	}
}

// Reviews implements board.Board.
func (b *Board) Reviews(ctx context.Context, repo string, number int) ([]board.Review, error) {
	latest := map[string]board.Review{}
	var order []string
	for page := 1; ; page++ {
		var reviews []struct {
			State string `json:"state"`
			User  struct {
				Login string `json:"login"`
				Type  string `json:"type"`
			} `json:"user"`
		}
		path := fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100&page=%d", repo, number, page)
		if err := b.rest.DoWithContext(ctx, "GET", path, nil, &reviews); err != nil {
			return nil, fmt.Errorf("read reviews of %s#%d: %w", repo, number, err)
		}
		for _, r := range reviews {
			// Comments do not change an earlier approval or change request.
			if r.State == "COMMENTED" {
				if _, seen := latest[r.User.Login]; seen {
					continue
				}
			}
			if _, seen := latest[r.User.Login]; !seen {
				order = append(order, r.User.Login)
			}
			latest[r.User.Login] = board.Review{Login: r.User.Login, Bot: r.User.Type != "User", State: r.State}
		}
		if len(reviews) < 100 {
			break
		}
	}
	out := make([]board.Review, 0, len(order))
	for _, login := range order {
		out = append(out, latest[login])
	}
	return out, nil
}
