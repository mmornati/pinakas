package cli

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/metrics"
)

func (app *App) defectCommand() *cobra.Command {
	var at, note string
	cmd := &cobra.Command{
		Use:   "defect <story> <bug>",
		Short: "Link a bug back to the story that introduced it",
		Long: "Post a defect marker on the story issue. `pinakas report` counts the defects found within 30 days of delivery, " +
			"so that fast but fragile stories show up. Both arguments are issue references (12, #12 or owner/name#12).",
		Example: "  pinakas defect 12 31\n  pinakas defect acme/app#12 acme/api#7 --at 2026-11-20 --note \"null session on logout\"",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			story, err := ws.ref(args[0])
			if err != nil {
				return err
			}
			bug, err := ws.ref(args[1])
			if err != nil {
				return err
			}
			if strings.EqualFold(story.Ref(), bug.Ref()) {
				return errors.New("a story cannot be its own defect")
			}
			found := app.Now().UTC()
			if at != "" {
				if found, err = parseTime(at, app.Now()); err != nil {
					return fmt.Errorf("--at: %w", err)
				}
			}
			d := metrics.Defect{
				Version: 1, Repo: story.Repo, Number: story.Number,
				DefectRepo: bug.Repo, DefectNumber: bug.Number, FoundAt: found.UTC(), Note: note,
			}
			body, err := d.Comment()
			if err != nil {
				return err
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			if err := b.Comment(cmd.Context(), story, body); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Linked %s to %s.\n", d.Ref(), story.Ref())
			return nil
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "when the bug was found (YYYY-MM-DD or RFC 3339; default now)")
	cmd.Flags().StringVar(&note, "note", "", "free-form note appended to the comment")
	return cmd
}

// ref resolves an issue reference, using the default repository for bare numbers.
func (ws *workspace) ref(s string) (board.Card, error) {
	repo, number, err := board.ParseRef(s)
	if err != nil {
		return board.Card{}, err
	}
	if repo == "" {
		repo = ws.cfg.Board.Repo
	}
	if repo == "" {
		return board.Card{}, fmt.Errorf("%s has no repository and board.repo is not set", s)
	}
	return board.Card{Repo: repo, Number: number}, nil
}

// parseTime reads YYYY-MM-DD (local midnight), RFC 3339, or a number of
// days back such as 30d.
func parseTime(s string, now time.Time) (time.Time, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err == nil && n >= 0 {
			return now.AddDate(0, 0, -n), nil
		}
	}
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not a date (YYYY-MM-DD), a time (RFC 3339) or a number of days (30d)", s)
}

const (
	reportTable    = "table"
	reportJSON     = "json"
	reportCSV      = "csv"
	reportMarkdown = "markdown"
)

func (app *App) reportCommand() *cobra.Command {
	var statuses []string
	var all, measured bool
	var since, format string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Measure delivered stories: duration, tokens, first pass, findings, defects at 30 days",
		Long: "Read the actuals, timing and defect markers on each card and print one line per story plus a summary. " +
			"By default only the done column is measured; use --status or --all to widen it. " +
			"The defect rate only counts stories whose 30-day window has ended.",
		Example: "  pinakas report\n  pinakas report --since 30d --format markdown >> $GITHUB_STEP_SUMMARY\n  pinakas report --all --measured --format csv > stories.csv",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch format {
			case reportTable, reportJSON, reportCSV, reportMarkdown:
			default:
				return fmt.Errorf("unknown --format %q (use table, json, csv or markdown)", format)
			}
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			f := metrics.Filter{Measured: measured}
			if !all {
				if len(statuses) == 0 {
					statuses = []string{config.ColumnDone}
				}
				for _, s := range statuses {
					f.Statuses = append(f.Statuses, ws.cfg.Column(s))
				}
			}
			if since != "" {
				if f.Since, err = parseTime(since, app.Now()); err != nil {
					return fmt.Errorf("--since: %w", err)
				}
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			stories, err := metrics.Collect(cmd.Context(), b, f, app.Now())
			if err != nil {
				return err
			}
			sum := metrics.Summarize(stories)
			out := cmd.OutOrStdout()
			switch format {
			case reportJSON:
				return encodeIndent(out, map[string]any{"summary": sum, "stories": nonNil(stories)})
			case reportCSV:
				return writeCSV(out, stories)
			case reportMarkdown:
				writeMarkdown(out, stories, sum)
				return nil
			default:
				return writeTable(out, stories, sum)
			}
		},
	}
	cmd.Flags().StringSliceVar(&statuses, "status", nil, "columns to measure (keys or names; default done)")
	cmd.Flags().BoolVar(&all, "all", false, "measure every card on the board")
	cmd.Flags().BoolVar(&measured, "measured", false, "only stories with actuals or timing")
	cmd.Flags().StringVar(&since, "since", "", "only stories delivered since (YYYY-MM-DD, RFC 3339 or 30d)")
	cmd.Flags().StringVar(&format, "format", reportTable, "table, json, csv or markdown")
	return cmd
}

func nonNil(s []metrics.Story) []metrics.Story {
	if s == nil {
		return []metrics.Story{}
	}
	return s
}

func encodeIndent(out io.Writer, v any) error {
	enc := newEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// row is one story rendered as text cells.
type row struct {
	ref, title, delivered, duration, tokens, firstPass, findings, agent, lead, review, defects string
}

func storyRow(s metrics.Story) row {
	r := row{ref: s.Ref(), title: s.Title, delivered: "-", duration: "-", tokens: "-", firstPass: "-", findings: "-", agent: "-", lead: "-", review: "-"}
	if s.DeliveredAt != nil {
		r.delivered = s.DeliveredAt.Local().Format(time.DateOnly)
	}
	if a := s.Actuals; a != nil {
		r.duration = a.DurationText()
		if a.Tokens != nil {
			r.tokens = strconv.Itoa(*a.Tokens)
		}
		if a.FirstPass != nil {
			r.firstPass = map[bool]string{true: "yes", false: "no"}[*a.FirstPass]
		}
		if len(a.Findings) > 0 {
			r.findings = metrics.FormatFindings(a.Findings)
		}
		if a.Agent != "" {
			r.agent = a.Agent
		}
	}
	if t := s.Timing; t != nil {
		r.review = minutesText(float64(t.ReviewMin))
		if t.LeadTimeMin != nil {
			r.lead = minutesText(float64(*t.LeadTimeMin))
		}
	}
	r.defects = strconv.Itoa(s.Defects30)
	if s.WindowOpen {
		r.defects += " (open)"
	}
	return r
}

func (r row) cells() []string {
	return []string{r.ref, r.title, r.delivered, r.duration, r.tokens, r.firstPass, r.findings, r.agent, r.lead, r.review, r.defects}
}

var reportHeader = []string{"Story", "Title", "Delivered", "Agent time", "Tokens", "First pass", "Findings", "Agent", "Lead time", "Review", "Defects 30d"}

func writeTable(out io.Writer, stories []metrics.Story, sum metrics.Summary) error {
	if len(stories) == 0 {
		fmt.Fprintln(out, "No stories match. Try --all, --status or a wider --since.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(reportHeader, "\t"))
	for _, s := range stories {
		r := storyRow(s)
		r.title = truncate(r.title, 40)
		fmt.Fprintln(w, strings.Join(r.cells(), "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	for _, l := range summaryLines(sum) {
		fmt.Fprintln(out, l[0]+": "+l[1])
	}
	return nil
}

func writeMarkdown(out io.Writer, stories []metrics.Story, sum metrics.Summary) {
	fmt.Fprintln(out, "## pinakas report")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| Measure | Value |")
	fmt.Fprintln(out, "|---|---|")
	for _, l := range summaryLines(sum) {
		fmt.Fprintf(out, "| %s | %s |\n", l[0], l[1])
	}
	if len(stories) == 0 {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| "+strings.Join(reportHeader, " | ")+" |")
	fmt.Fprintln(out, "|"+strings.Repeat("---|", len(reportHeader)))
	for _, s := range stories {
		cells := storyRow(s).cells()
		for i, c := range cells {
			cells[i] = strings.ReplaceAll(c, "|", `\|`)
		}
		fmt.Fprintln(out, "| "+strings.Join(cells, " | ")+" |")
	}
}

func writeCSV(out io.Writer, stories []metrics.Story) error {
	w := csv.NewWriter(out)
	_ = w.Write([]string{"repo", "number", "title", "status", "delivered_at", "duration_min", "tokens", "first_pass", "findings", "agent", "build_min", "review_min", "lead_time_min", "defects_30d", "window_open", "duration_sec"})
	for _, s := range stories {
		rec := []string{s.Repo, strconv.Itoa(s.Number), s.Title, s.Status, "", "", "", "", "", "", "", "", "", strconv.Itoa(s.Defects30), strconv.FormatBool(s.WindowOpen), ""}
		if s.DeliveredAt != nil {
			rec[4] = s.DeliveredAt.UTC().Format(time.RFC3339)
		}
		if a := s.Actuals; a != nil {
			rec[5] = strconv.Itoa(a.DurationMin)
			if a.Tokens != nil {
				rec[6] = strconv.Itoa(*a.Tokens)
			}
			if a.FirstPass != nil {
				rec[7] = strconv.FormatBool(*a.FirstPass)
			}
			rec[8] = metrics.FormatFindings(a.Findings)
			rec[9] = a.Agent
			if a.DurationSec > 0 || a.DurationMin == 0 {
				rec[15] = strconv.Itoa(a.DurationSec)
			}
		}
		if t := s.Timing; t != nil {
			if t.BuildMin != nil {
				rec[10] = strconv.Itoa(*t.BuildMin)
			}
			rec[11] = strconv.Itoa(t.ReviewMin)
			if t.LeadTimeMin != nil {
				rec[12] = strconv.Itoa(*t.LeadTimeMin)
			}
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return w.Error()
}

func summaryLines(s metrics.Summary) [][2]string {
	opt := func(v *float64, f func(float64) string) string {
		if v == nil {
			return "-"
		}
		return f(*v)
	}
	pct := func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }
	findings := metrics.FormatFindings(s.Findings)
	if findings == "" {
		findings = "-"
	}
	defectRate := opt(s.DefectRate30, pct)
	if s.DefectRate30 != nil {
		defectRate += fmt.Sprintf(" of %d stories with a closed window", s.StoriesClosed30)
	}
	return [][2]string{
		{"Stories", fmt.Sprintf("%d (%d with actuals)", s.Stories, s.WithActuals)},
		{"Median agent time", opt(s.MedianDuration, agentText)},
		{"Total agent time", agentTotal(s)},
		{"Total tokens", strconv.Itoa(s.TotalTokens)},
		{"First-pass rate", opt(s.FirstPassRate, pct)},
		{"Review findings", findings},
		{"Median lead time", opt(s.MedianLeadTime, minutesText)},
		{"Median review time", opt(s.MedianReview, minutesText)},
		{"Defects within 30 days", strconv.Itoa(s.Defects30)},
		{"Stories with a defect", defectRate},
	}
}

// agentText renders agent time like the story rows: a recorded duration
// under a minute, zero seconds included, reads "< 1 min".
func agentText(m float64) string {
	if m < 1 {
		return "< 1 min"
	}
	return minutesText(m)
}

func agentTotal(s metrics.Summary) string {
	if s.WithActuals == 0 {
		return "-"
	}
	return agentText(s.TotalDuration)
}

// minutesText renders minutes as "< 1 min", "45 min", "2 h 05" or "3 d 4 h".
func minutesText(m float64) string {
	total := int(m + 0.5)
	switch {
	case m > 0 && m < 1:
		return "< 1 min"
	case total < 60:
		return fmt.Sprintf("%d min", total)
	case total < 24*60:
		return fmt.Sprintf("%d h %02d", total/60, total%60)
	default:
		return fmt.Sprintf("%d d %d h", total/(24*60), total%(24*60)/60)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
