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
		build, lead := minutes(claimedAt, openedAt), minutes(claimedAt, mergedAt)
		t.ClaimedAt, t.BuildMin, t.LeadTimeMin = &c, &build, &lead
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
	return b.String(), nil
}

// HasTiming reports whether a comment already records timing for a PR.
func HasTiming(body string, prNumber int) bool {
	_, rest, ok := strings.Cut(body, TimingMarker)
	if !ok {
		return false
	}
	payload, _, ok := strings.Cut(rest, " -->")
	if !ok {
		return false
	}
	var t Timing
	return json.Unmarshal([]byte(payload), &t) == nil && t.PR == prNumber
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
