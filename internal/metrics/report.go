package metrics

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/pr"
)

// DefectWindow is how long after delivery a defect counts against a story.
const DefectWindow = 30 * 24 * time.Hour

// Story is everything measured about one card.
type Story struct {
	Repo    string           `json:"repo"`
	Number  int              `json:"number"`
	Title   string           `json:"title"`
	Status  string           `json:"status"`
	Actuals *actuals.Actuals `json:"actuals,omitempty"`
	Timing  *pr.Timing       `json:"timing,omitempty"`
	Defects []Defect         `json:"defects,omitempty"`
	// DeliveredAt is the merge time, or the actuals time without a merge.
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	// Defects30 counts defects found within DefectWindow of delivery.
	Defects30 int `json:"defects_30d"`
	// WindowOpen is true while the 30-day window has not ended yet.
	WindowOpen bool `json:"window_open"`
}

// Ref returns owner/name#number.
func (s Story) Ref() string { return fmt.Sprintf("%s#%d", s.Repo, s.Number) }

// Filter selects the cards to measure.
type Filter struct {
	// Statuses keeps cards in these columns; empty keeps every card.
	Statuses []string
	// Since keeps stories delivered at or after this time; zero keeps all.
	Since time.Time
	// Measured keeps only stories with actuals or timing.
	Measured bool
}

// Collect reads the markers on each card and builds the stories.
func Collect(ctx context.Context, b board.Board, f Filter, now time.Time) ([]Story, error) {
	cards, err := b.Cards(ctx)
	if err != nil {
		return nil, err
	}
	var stories []Story
	for _, c := range cards {
		if len(f.Statuses) > 0 && !slices.Contains(f.Statuses, c.Status) {
			continue
		}
		comments, err := b.Comments(ctx, c.Repo, c.Number)
		if err != nil {
			return nil, err
		}
		s := build(c, comments, now)
		if f.Measured && s.Actuals == nil && s.Timing == nil {
			continue
		}
		if !f.Since.IsZero() && (s.DeliveredAt == nil || s.DeliveredAt.Before(f.Since)) {
			continue
		}
		stories = append(stories, s)
	}
	slices.SortStableFunc(stories, func(a, b Story) int {
		switch {
		case a.DeliveredAt == nil && b.DeliveredAt == nil:
			return 0
		case a.DeliveredAt == nil:
			return 1
		case b.DeliveredAt == nil:
			return -1
		}
		return a.DeliveredAt.Compare(*b.DeliveredAt)
	})
	return stories, nil
}

func build(c board.Card, comments []string, now time.Time) Story {
	s := Story{Repo: c.Repo, Number: c.Number, Title: c.Title, Status: c.Status}
	for _, body := range comments {
		if a, ok := actuals.Parse(body); ok && a.Number == c.Number {
			s.Actuals = &a // the latest actuals win
		}
		if t, ok := pr.ParseTiming(body); ok && t.Number == c.Number {
			s.Timing = &t
		}
		if d, ok := ParseDefect(body); ok && d.Number == c.Number {
			s.Defects = append(s.Defects, d)
		}
	}
	switch {
	case s.Timing != nil:
		at := s.Timing.MergedAt
		s.DeliveredAt = &at
	case s.Actuals != nil:
		at := s.Actuals.FinishedAt
		s.DeliveredAt = &at
	}
	if s.DeliveredAt != nil {
		end := s.DeliveredAt.Add(DefectWindow)
		s.WindowOpen = now.Before(end)
		for _, d := range s.Defects {
			if !d.FoundAt.Before(*s.DeliveredAt) && !d.FoundAt.After(end) {
				s.Defects30++
			}
		}
	}
	return s
}

// Summary aggregates stories so that two experiments can be compared.
type Summary struct {
	Stories         int            `json:"stories"`
	WithActuals     int            `json:"with_actuals"`
	MedianDuration  *float64       `json:"median_duration_min,omitempty"`
	TotalDuration   float64        `json:"total_duration_min"`
	TotalTokens     int            `json:"total_tokens"`
	FirstPassRate   *float64       `json:"first_pass_rate,omitempty"`
	Findings        map[string]int `json:"findings"`
	MedianLeadTime  *float64       `json:"median_lead_time_min,omitempty"`
	MedianReview    *float64       `json:"median_review_min,omitempty"`
	Defects30       int            `json:"defects_30d"`
	StoriesClosed30 int            `json:"stories_with_closed_window"`
	DefectRate30    *float64       `json:"defect_rate_30d,omitempty"`
}

// Summarize computes the aggregates.
func Summarize(stories []Story) Summary {
	sum := Summary{Stories: len(stories), Findings: map[string]int{}}
	var durations, leads, reviews []float64
	passKnown, passed, withDefects := 0, 0, 0
	for _, s := range stories {
		if a := s.Actuals; a != nil {
			sum.WithActuals++
			durations = append(durations, a.Minutes())
			sum.TotalDuration += a.Minutes()
			if a.Tokens != nil {
				sum.TotalTokens += *a.Tokens
			}
			if a.FirstPass != nil {
				passKnown++
				if *a.FirstPass {
					passed++
				}
			}
			for k, v := range a.Findings {
				sum.Findings[k] += v
			}
		}
		if t := s.Timing; t != nil {
			reviews = append(reviews, float64(t.ReviewMin))
			if t.LeadTimeMin != nil {
				leads = append(leads, float64(*t.LeadTimeMin))
			}
		}
		sum.Defects30 += s.Defects30
		if s.DeliveredAt != nil && !s.WindowOpen {
			sum.StoriesClosed30++
			if s.Defects30 > 0 {
				withDefects++
			}
		}
	}
	sum.MedianDuration = median(durations)
	sum.MedianLeadTime = median(leads)
	sum.MedianReview = median(reviews)
	if passKnown > 0 {
		r := float64(passed) / float64(passKnown)
		sum.FirstPassRate = &r
	}
	if sum.StoriesClosed30 > 0 {
		r := float64(withDefects) / float64(sum.StoriesClosed30)
		sum.DefectRate30 = &r
	}
	return sum
}

func median(v []float64) *float64 {
	if len(v) == 0 {
		return nil
	}
	s := slices.Clone(v)
	slices.Sort(s)
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	return &m
}

// FormatFindings renders "major=1, minor=3" in a stable order.
func FormatFindings(f map[string]int) string {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, f[k]))
	}
	return strings.Join(parts, ", ")
}
