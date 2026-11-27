package pr

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/config"
)

// Sync moves the cards a pull request closes, following its lifecycle:
// opened or ready for review → review, back to draft → in progress,
// merged → done with a timing comment. It returns one line per action.
func Sync(ctx context.Context, cfg config.Config, b board.Board, ev Event) ([]string, error) {
	p := ev.PullRequest
	links := LinkedIssues(p.Body, ev.Repository.FullName)
	if len(links) == 0 {
		return []string{fmt.Sprintf("PR #%d closes no issue: nothing to move (add \"Closes #N\" to the description).", p.Number)}, nil
	}

	var target string
	switch {
	case ev.Action == "closed" && p.Merged:
		target = cfg.Column(config.ColumnDone)
	case ev.Action == "closed":
		return []string{fmt.Sprintf("PR #%d closed without merge: cards left where they are.", p.Number)}, nil
	case ev.Action == "converted_to_draft":
		target = cfg.Column(config.ColumnInProgress)
	case p.Draft:
		return []string{fmt.Sprintf("PR #%d is a draft: cards stay in progress.", p.Number)}, nil
	case slices.Contains([]string{"opened", "reopened", "ready_for_review", "edited"}, ev.Action):
		target = cfg.Column(config.ColumnReview)
	default:
		return []string{fmt.Sprintf("Nothing to do for pull_request %q.", ev.Action)}, nil
	}

	cards, err := b.Cards(ctx)
	if err != nil {
		return nil, err
	}
	var lines []string
	var errs []error
	for _, ref := range links {
		card, err := board.Find(cards, ref.Repo, ref.Number)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s is not on the board: skipped.", ref))
			continue
		}
		if card.Status == target {
			lines = append(lines, fmt.Sprintf("%s already in %q.", ref, target))
		} else if card.Status == cfg.Column(config.ColumnDone) && target != cfg.Column(config.ColumnDone) {
			lines = append(lines, fmt.Sprintf("%s is done: not moved back to %q.", ref, target))
			continue
		} else if err := b.Move(ctx, card, target); err != nil {
			errs = append(errs, err)
			continue
		} else {
			lines = append(lines, fmt.Sprintf("%s moved from %q to %q.", ref, card.Status, target))
		}
		if ev.Action == "closed" && p.Merged {
			line, err := recordTiming(ctx, b, card, ref, p)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			lines = append(lines, line)
		}
	}
	return lines, errors.Join(errs...)
}

func recordTiming(ctx context.Context, b board.Board, card board.Card, ref IssueRef, p PullRequest) (string, error) {
	comments, err := b.Comments(ctx, ref.Repo, ref.Number)
	if err != nil {
		return "", err
	}
	var claimedAt time.Time
	for _, body := range comments {
		if HasTiming(body, p.Number) {
			return fmt.Sprintf("%s timing already recorded for PR #%d.", ref, p.Number), nil
		}
		if a, ok := actuals.Parse(body); ok && a.Number == ref.Number && strings.EqualFold(a.Repo, ref.Repo) {
			claimedAt = a.ClaimedAt
		}
	}
	if claimedAt.IsZero() {
		if claimedAt, err = b.FirstAssignedAt(ctx, ref.Repo, ref.Number); err != nil {
			return "", err
		}
	}
	mergedAt := time.Now()
	if p.MergedAt != nil {
		mergedAt = *p.MergedAt
	}
	t := NewTiming(ref, p.Number, claimedAt, p.CreatedAt, mergedAt)
	body, err := t.Comment()
	if err != nil {
		return "", err
	}
	if err := b.Comment(ctx, card, body); err != nil {
		return "", err
	}
	if t.LeadTimeMin != nil {
		return fmt.Sprintf("%s timing recorded: %s from claim to merge.", ref, human(*t.LeadTimeMin)), nil
	}
	return fmt.Sprintf("%s timing recorded (claim time unknown).", ref), nil
}
