// Package board defines the backend-neutral view of a Kanban board.
package board

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Card is one issue on the board.
type Card struct {
	ItemID    string   `json:"item_id"`
	Repo      string   `json:"repo"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Assignees []string `json:"assignees"`
}

// Ref returns owner/name#number.
func (c Card) Ref() string { return fmt.Sprintf("%s#%d", c.Repo, c.Number) }

// Board is implemented by each backend (GitHub Projects in v0.1).
type Board interface {
	// Cards lists the issues on the board.
	Cards(ctx context.Context) ([]Card, error)
	// Move sets the card's status column to the given option name.
	Move(ctx context.Context, card Card, status string) error
	// Assign adds the user to the card's assignees.
	Assign(ctx context.Context, card Card, login string) error
	// Comment adds a comment to the card's issue.
	Comment(ctx context.Context, card Card, body string) error
	// CurrentUser returns the login the backend is authenticated as.
	CurrentUser(ctx context.Context) (string, error)
}

// ErrCardNotFound is returned when an issue is not on the board.
var ErrCardNotFound = errors.New("card not found on the board")

// Find returns the card for repo#number. When repo is empty, the number
// must be unique across the board.
func Find(cards []Card, repo string, number int) (Card, error) {
	var matches []Card
	for _, c := range cards {
		if c.Number == number && (repo == "" || strings.EqualFold(c.Repo, repo)) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		if repo == "" {
			return Card{}, fmt.Errorf("#%d: %w", number, ErrCardNotFound)
		}
		return Card{}, fmt.Errorf("%s#%d: %w", repo, number, ErrCardNotFound)
	case 1:
		return matches[0], nil
	}
	refs := make([]string, len(matches))
	for i, c := range matches {
		refs[i] = c.Ref()
	}
	return Card{}, fmt.Errorf("#%d is ambiguous (%s): set board.repo or pass owner/name#%d", number, strings.Join(refs, ", "), number)
}

// ParseRef reads "42", "#42" or "owner/name#42".
func ParseRef(s string) (repo string, number int, err error) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "#"); i >= 0 {
		repo, s = s[:i], s[i+1:]
	}
	if _, err := fmt.Sscanf(s, "%d", &number); err != nil || number <= 0 || fmt.Sprint(number) != s {
		return "", 0, fmt.Errorf("invalid issue reference %q: use 42 or owner/name#42", s)
	}
	if repo != "" && strings.Count(repo, "/") != 1 {
		return "", 0, fmt.Errorf("invalid repository %q in issue reference", repo)
	}
	return repo, number, nil
}
