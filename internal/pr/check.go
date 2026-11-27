package pr

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/config"
)

// Result is the outcome of the required CI check.
type Result struct {
	Pass  bool
	Lines []string
}

func (r *Result) ok(format string, args ...any) {
	r.Lines = append(r.Lines, "✅ "+fmt.Sprintf(format, args...))
}

func (r *Result) fail(format string, args ...any) {
	r.Pass = false
	r.Lines = append(r.Lines, "❌ "+fmt.Sprintf(format, args...))
}

// Check verifies that a pull request follows the board workflow: it closes
// a card of the board, the card has actuals, and a risky change has a
// human approval. It is the server-side guardrail: it does not depend on
// which agent opened the pull request.
//
// A failed API call fails the check with a line that says what could not
// be verified, and the other rules are still reported.
func Check(ctx context.Context, cfg config.Config, b board.Board, ev Event) Result {
	p := ev.PullRequest
	res := Result{Pass: true}
	if slices.Contains(cfg.Rules.ExemptAuthors, p.User.Login) {
		res.ok("author %s is exempt from the board workflow", p.User.Login)
		return res
	}

	links := LinkedIssues(p.Body, ev.Repository.FullName)
	switch {
	case len(links) == 0 && cfg.Rules.RequireLinkedIssue:
		res.fail("the description closes no issue: add \"Closes #N\" for the card this pull request delivers")
	case len(links) == 0:
		res.ok("no linked issue (not required)")
	}

	if len(links) > 0 {
		cards, err := b.Cards(ctx)
		if err != nil {
			res.fail("could not read the board: %v", err)
		}
		for _, ref := range links {
			if err != nil {
				break
			}
			if _, err := board.Find(cards, ref.Repo, ref.Number); err != nil {
				res.fail("%s is not on the board: add it to the project, or link the right issue", ref)
				continue
			}
			res.ok("%s is on the board", ref)
			if !cfg.Rules.RequireActualsOnPR {
				continue
			}
			has, aerr := hasActuals(ctx, b, ref)
			switch {
			case aerr != nil:
				res.fail("could not read the comments of %s: %v", ref, aerr)
			case has:
				res.ok("%s has actuals", ref)
			default:
				res.fail("%s has no actuals: run `pinakas actuals %d` from the working tree that claimed it", ref, ref.Number)
			}
		}
	}

	if label, risky := p.HasLabel(cfg.Rules.ApprovalLabels); risky {
		reviews, err := b.Reviews(ctx, ev.Repository.FullName, p.Number)
		if err != nil {
			res.fail("label %q needs a human approval, but the reviews could not be read: %v", label, err)
			return res
		}
		var approvers []string
		for _, r := range reviews {
			if r.State == "APPROVED" && !r.Bot && !strings.EqualFold(r.Login, p.User.Login) {
				approvers = append(approvers, r.Login)
			}
		}
		if len(approvers) == 0 {
			res.fail("label %q needs an approving review from a human other than the author", label)
		} else {
			res.ok("label %q approved by %s", label, strings.Join(approvers, ", "))
		}
	}
	return res
}

func hasActuals(ctx context.Context, b board.Board, ref IssueRef) (bool, error) {
	comments, err := b.Comments(ctx, ref.Repo, ref.Number)
	if err != nil {
		return false, err
	}
	for _, body := range comments {
		if a, ok := actuals.Parse(body); ok && a.Number == ref.Number {
			return true, nil
		}
	}
	return false, nil
}
