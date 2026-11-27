// Package pr ties pull requests to board cards for the GitHub Action:
// moving cards on pull request events, measuring durations from
// timestamps, and the required CI check.
package pr

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event is the subset of a pull_request or pull_request_review webhook
// payload that pinakas reads.
type Event struct {
	Action      string      `json:"action"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// PullRequest is the pull request inside an Event.
type PullRequest struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	HTMLURL   string     `json:"html_url"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// ParseEvent decodes a webhook payload (the file at GITHUB_EVENT_PATH).
func ParseEvent(r io.Reader) (Event, error) {
	var e Event
	if err := json.NewDecoder(r).Decode(&e); err != nil {
		return e, fmt.Errorf("decode event: %w", err)
	}
	if e.PullRequest.Number == 0 {
		return e, fmt.Errorf("event has no pull request: run on pull_request or pull_request_review")
	}
	return e, nil
}

// HasLabel reports whether the pull request carries one of the labels.
func (p PullRequest) HasLabel(names []string) (string, bool) {
	for _, l := range p.Labels {
		for _, n := range names {
			if strings.EqualFold(l.Name, n) {
				return l.Name, true
			}
		}
	}
	return "", false
}

// IssueRef is an issue a pull request closes.
type IssueRef struct {
	Repo   string
	Number int
}

func (r IssueRef) String() string { return fmt.Sprintf("%s#%d", r.Repo, r.Number) }

// closing matches GitHub's closing keywords: "Closes #12", "fixes: acme/app#3",
// "Resolves https://github.com/acme/app/issues/4".
var closing = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\b:?\s+(?:([\w.-]+/[\w.-]+)?#(\d+)|https://github\.com/([\w.-]+/[\w.-]+)/issues/(\d+))`)

// LinkedIssues returns the issues closed by the pull request body, in
// order and without duplicates. Bare #N refers to defaultRepo.
func LinkedIssues(body, defaultRepo string) []IssueRef {
	var out []IssueRef
	seen := map[string]bool{}
	for _, m := range closing.FindAllStringSubmatch(body, -1) {
		repo, num := m[1], m[2]
		if num == "" {
			repo, num = m[3], m[4]
		}
		if repo == "" {
			repo = defaultRepo
		}
		n, err := strconv.Atoi(num)
		if err != nil || n <= 0 {
			continue
		}
		ref := IssueRef{Repo: repo, Number: n}
		key := strings.ToLower(ref.String())
		if !seen[key] {
			seen[key] = true
			out = append(out, ref)
		}
	}
	return out
}
