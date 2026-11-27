// Package boardtest provides an in-memory board for tests.
package boardtest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mmornati/pinakas/internal/board"
)

// Fake is an in-memory board.Board that records every write.
type Fake struct {
	CardList   []board.Card
	User       string
	Moves      []string
	Assigned   []string
	Unassigned []string
	Posted     []string            // comment bodies written through Comment
	IssueNotes map[string][]string // existing comments by "owner/name#n"
	Assigned1  map[string]time.Time
	ReviewList map[string][]board.Review // by "owner/name#n"
	ReviewsErr error                     // returned by Reviews when set
}

func key(repo string, number int) string { return strings.ToLower(fmt.Sprintf("%s#%d", repo, number)) }

// Cards implements board.Board.
func (f *Fake) Cards(context.Context) ([]board.Card, error) { return f.CardList, nil }

// CurrentUser implements board.Board.
func (f *Fake) CurrentUser(context.Context) (string, error) { return f.User, nil }

// Move implements board.Board.
func (f *Fake) Move(_ context.Context, c board.Card, status string) error {
	f.Moves = append(f.Moves, c.Ref()+"->"+status)
	for i := range f.CardList {
		if f.CardList[i].Number == c.Number && f.CardList[i].Repo == c.Repo {
			f.CardList[i].Status = status
		}
	}
	return nil
}

// Assign implements board.Board.
func (f *Fake) Assign(_ context.Context, c board.Card, login string) error {
	f.Assigned = append(f.Assigned, c.Ref()+"="+login)
	for i := range f.CardList {
		if f.CardList[i].Number == c.Number && f.CardList[i].Repo == c.Repo {
			f.CardList[i].Assignees = append(f.CardList[i].Assignees, login)
		}
	}
	return nil
}

// Unassign implements board.Board.
func (f *Fake) Unassign(_ context.Context, c board.Card, login string) error {
	f.Unassigned = append(f.Unassigned, c.Ref()+"="+login)
	for i := range f.CardList {
		if f.CardList[i].Number == c.Number && f.CardList[i].Repo == c.Repo {
			f.CardList[i].Assignees = slices.DeleteFunc(f.CardList[i].Assignees, func(l string) bool { return l == login })
		}
	}
	return nil
}

// Comment implements board.Board. Posted comments are also returned by Comments.
func (f *Fake) Comment(_ context.Context, c board.Card, body string) error {
	f.Posted = append(f.Posted, body)
	if f.IssueNotes == nil {
		f.IssueNotes = map[string][]string{}
	}
	k := key(c.Repo, c.Number)
	f.IssueNotes[k] = append(f.IssueNotes[k], body)
	return nil
}

// Comments implements board.Board.
func (f *Fake) Comments(_ context.Context, repo string, number int) ([]string, error) {
	return f.IssueNotes[key(repo, number)], nil
}

// FirstAssignedAt implements board.Board.
func (f *Fake) FirstAssignedAt(_ context.Context, repo string, number int) (time.Time, error) {
	return f.Assigned1[key(repo, number)], nil
}

// Reviews implements board.Board.
func (f *Fake) Reviews(_ context.Context, repo string, number int) ([]board.Review, error) {
	if f.ReviewsErr != nil {
		return nil, f.ReviewsErr
	}
	return f.ReviewList[key(repo, number)], nil
}
