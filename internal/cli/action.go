package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmornati/pinakas/internal/pr"
)

// readEvent loads the webhook payload from --event or GITHUB_EVENT_PATH.
func readEvent(path string) (pr.Event, error) {
	if path == "" {
		path = os.Getenv("GITHUB_EVENT_PATH")
	}
	if path == "" {
		return pr.Event{}, errors.New("no event: pass --event or run inside GitHub Actions")
	}
	f, err := os.Open(path)
	if err != nil {
		return pr.Event{}, err
	}
	defer func() { _ = f.Close() }()
	return pr.ParseEvent(f)
}

// writeSummary appends markdown to the job summary when running in Actions.
func writeSummary(title string, lines []string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintf(f, "### %s\n\n", title)
	for _, l := range lines {
		fmt.Fprintf(f, "- %s\n", l)
	}
	fmt.Fprintln(f)
}

func printLines(w io.Writer, lines []string) {
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
}

func (app *App) syncPRCommand() *cobra.Command {
	var eventPath string
	cmd := &cobra.Command{
		Use:   "sync-pr",
		Short: "Action: move the cards a pull request closes and record timing at merge",
		Long: "Reads a pull_request event (GITHUB_EVENT_PATH). Opened or ready for review moves the linked cards to review, " +
			"back to draft moves them to in progress, merged moves them to done and posts a timing comment computed from timestamps.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			ev, err := readEvent(eventPath)
			if err != nil {
				return err
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			lines, err := pr.Sync(ctx, ws.cfg, b, ev)
			printLines(cmd.OutOrStdout(), lines)
			writeSummary("pinakas board sync", lines)
			return err
		},
	}
	cmd.Flags().StringVar(&eventPath, "event", "", "path to the webhook payload (default: GITHUB_EVENT_PATH)")
	return cmd
}

func (app *App) checkCICommand() *cobra.Command {
	var eventPath string
	cmd := &cobra.Command{
		Use:   "check-ci",
		Short: "Action: required check that a pull request follows the board workflow",
		Long: "Reads a pull_request or pull_request_review event. Fails unless the pull request closes a card of the board, " +
			"the card has actuals, and any risk label listed in rules.approval_labels has a human approval.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			ev, err := readEvent(eventPath)
			if err != nil {
				return err
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			res := pr.Check(ctx, ws.cfg, b, ev)
			printLines(cmd.OutOrStdout(), res.Lines)
			writeSummary("pinakas workflow check", res.Lines)
			if !res.Pass {
				return &exitError{code: 1, msg: "pinakas: pull request #" + fmt.Sprint(ev.PullRequest.Number) + " does not follow the board workflow:\n" + failures(res.Lines)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&eventPath, "event", "", "path to the webhook payload (default: GITHUB_EVENT_PATH)")
	return cmd
}

func failures(lines []string) string {
	var out []string
	for _, l := range lines {
		if strings.HasPrefix(l, "❌") {
			out = append(out, "  "+l)
		}
	}
	return strings.Join(out, "\n")
}
