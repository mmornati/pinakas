// Package actuals records what a story really cost.
//
// Every story gets the same fields so that experiments can be compared:
// timestamped duration, tokens, first-pass success and review findings by
// severity. Defects found later are linked with `pinakas defect`.
package actuals

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Marker opens the machine-readable part of an actuals comment.
const Marker = "<!-- pinakas:actuals "

// Actuals is the measurement for one story.
type Actuals struct {
	Version     int            `json:"v"`
	Repo        string         `json:"repo"`
	Number      int            `json:"number"`
	ClaimedAt   time.Time      `json:"claimed_at"`
	FinishedAt  time.Time      `json:"finished_at"`
	DurationMin int            `json:"duration_min"`
	DurationSec int            `json:"duration_sec"`
	Tokens      *int           `json:"tokens,omitempty"`
	FirstPass   *bool          `json:"first_pass,omitempty"`
	Findings    map[string]int `json:"findings,omitempty"`
	Agent       string         `json:"agent,omitempty"`
	Note        string         `json:"note,omitempty"`
}

// New computes the duration from the claim and finish timestamps.
func New(repo string, number int, claimedAt, finishedAt time.Time) Actuals {
	minutes := int(math.Round(finishedAt.Sub(claimedAt).Minutes()))
	return Actuals{
		Version:     1,
		Repo:        repo,
		Number:      number,
		ClaimedAt:   claimedAt.UTC(),
		FinishedAt:  finishedAt.UTC(),
		DurationMin: max(minutes, 0),
		DurationSec: max(int(finishedAt.Sub(claimedAt).Seconds()), 0),
	}
}

// DurationText is the duration for people: "< 1 min" rather than "0 min"
// for a story that took seconds.
func (a Actuals) DurationText() string {
	if a.DurationMin == 0 && a.DurationSec > 0 {
		return "< 1 min"
	}
	return fmt.Sprintf("%d min", a.DurationMin)
}

// ParseFirstPass accepts yes/no/true/false; an empty string means unknown.
func ParseFirstPass(s string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return nil, nil
	case "yes", "y", "true":
		v := true
		return &v, nil
	case "no", "n", "false":
		v := false
		return &v, nil
	}
	return nil, fmt.Errorf("first-pass must be yes or no, got %q", s)
}

var findingKey = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// ParseFindings reads "critical=0,major=1,minor=3".
func ParseFindings(s string) (map[string]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]int{}
	for part := range strings.SplitSeq(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		k = strings.ToLower(strings.TrimSpace(k))
		if !ok || !findingKey.MatchString(k) {
			return nil, fmt.Errorf("findings must look like major=1,minor=3, got %q", part)
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("findings count for %q must be a non-negative integer", k)
		}
		out[k] = n
	}
	return out, nil
}

// Comment renders the issue comment: a readable table plus a JSON marker
// that `pinakas report` (v0.3) and CI checks can parse back.
func (a Actuals) Comment() (string, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s -->\n", Marker, raw)
	b.WriteString("### pinakas actuals\n\n| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Claimed | %s |\n", a.ClaimedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "| Finished | %s |\n", a.FinishedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "| Duration | %s |\n", a.DurationText())
	if a.Tokens != nil {
		fmt.Fprintf(&b, "| Tokens | %d |\n", *a.Tokens)
	}
	if a.FirstPass != nil {
		fmt.Fprintf(&b, "| First pass | %s |\n", yesNo(*a.FirstPass))
	}
	if len(a.Findings) > 0 {
		fmt.Fprintf(&b, "| Findings | %s |\n", formatFindings(a.Findings))
	}
	if a.Agent != "" {
		fmt.Fprintf(&b, "| Agent | %s |\n", a.Agent)
	}
	if a.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", a.Note)
	}
	return b.String(), nil
}

// Parse extracts actuals from a comment body written by Comment.
func Parse(body string) (Actuals, bool) {
	_, rest, ok := strings.Cut(body, Marker)
	if !ok {
		return Actuals{}, false
	}
	payload, _, ok := strings.Cut(rest, " -->")
	if !ok {
		return Actuals{}, false
	}
	var a Actuals
	if err := json.Unmarshal([]byte(payload), &a); err != nil {
		return Actuals{}, false
	}
	return a, true
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatFindings(f map[string]int) string {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, f[k])
	}
	return strings.Join(parts, ", ")
}
