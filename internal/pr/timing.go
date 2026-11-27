package pr

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// TimingMarker opens the machine-readable part of a timing comment.
const TimingMarker = "<!-- pinakas:timing "

// Timing is measured by the Action from timestamps only: it does not
// depend on what the agent reports.
type Timing struct {
	Version     int        `json:"v"`
	Repo        string     `json:"repo"`
	Number      int        `json:"number"`
	PR          int        `json:"pr"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	OpenedAt    time.Time  `json:"pr_opened_at"`
	MergedAt    time.Time  `json:"merged_at"`
	BuildMin    *int       `json:"build_min,omitempty"`
	ReviewMin   int        `json:"review_min"`
	LeadTimeMin *int       `json:"lead_time_min,omitempty"`
	// ClaimedAfterPR is set when the card was claimed after the pull
	// request was opened (rework, or a card claimed late): the claim to
	// pull request step is then left out instead of being shown as 0.
	ClaimedAfterPR bool `json:"claimed_after_pr,omitempty"`
}

// NewTiming computes the durations. claimedAt may be zero when unknown.
func NewTiming(issue IssueRef, prNumber int, claimedAt, openedAt, mergedAt time.Time) Timing {
	t := Timing{
		Version:   1,
		Repo:      issue.Repo,
		Number:    issue.Number,
		PR:        prNumber,
		OpenedAt:  openedAt.UTC(),
		MergedAt:  mergedAt.UTC(),
		ReviewMin: minutes(openedAt, mergedAt),
	}
	if !claimedAt.IsZero() {
		c := claimedAt.UTC()
		lead := minutes(claimedAt, mergedAt)
		t.ClaimedAt, t.LeadTimeMin = &c, &lead
		if claimedAt.After(openedAt) {
			t.ClaimedAfterPR = true
		} else {
			build := minutes(claimedAt, openedAt)
			t.BuildMin = &build
		}
	}
	return t
}

func minutes(from, to time.Time) int {
	return max(int(math.Round(to.Sub(from).Minutes())), 0)
}

// Comment renders the timing comment posted on the issue at merge.
func (t Timing) Comment() (string, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s -->\n", TimingMarker, raw)
	fmt.Fprintf(&b, "### pinakas timing (PR #%d merged)\n\n| Step | Duration |\n| --- | --- |\n", t.PR)
	if t.BuildMin != nil {
		fmt.Fprintf(&b, "| Claim to pull request | %s |\n", human(*t.BuildMin))
	}
	fmt.Fprintf(&b, "| Pull request to merge | %s |\n", human(t.ReviewMin))
	if t.LeadTimeMin != nil {
		fmt.Fprintf(&b, "| Claim to merge | %s |\n", human(*t.LeadTimeMin))
	} else {
		b.WriteString("\nClaim time unknown: the card was never claimed or assigned.\n")
	}
	if t.ClaimedAfterPR {
		b.WriteString("\nThe card was claimed after the pull request was opened, so claim to pull request is not measured.\n")
	}
	return b.String(), nil
}

// HasTiming reports whether a comment already records timing for a PR.
func HasTiming(body string, prNumber int) bool {
	t, ok := ParseTiming(body)
	return ok && t.PR == prNumber
}

// ParseTiming extracts timing from a comment body written by Comment.
func ParseTiming(body string) (Timing, bool) {
	_, rest, ok := strings.Cut(body, TimingMarker)
	if !ok {
		return Timing{}, false
	}
	payload, _, ok := strings.Cut(rest, " -->")
	if !ok {
		return Timing{}, false
	}
	var t Timing
	if json.Unmarshal([]byte(payload), &t) != nil {
		return Timing{}, false
	}
	return t, true
}

func human(total int) string {
	if total < 60 {
		return fmt.Sprintf("%d min", total)
	}
	if total%60 == 0 {
		return fmt.Sprintf("%d h", total/60)
	}
	return fmt.Sprintf("%d h %02d min", total/60, total%60)
}
