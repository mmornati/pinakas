package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/hooks"
	"github.com/mmornati/pinakas/internal/state"
	"github.com/mmornati/pinakas/internal/templates"
	"github.com/mmornati/pinakas/plugin"
)

const configTemplate = `# pinakas configuration — https://github.com/mmornati/pinakas
board:
  provider: github
  # User or organization that owns the GitHub Project (v2).
  owner: %s
  # Project number, as in https://github.com/users/<owner>/projects/<number>.
  project: %d
  # Default repository for bare issue numbers (owner/name).
  repo: %s
  # Single-select field that holds the column.
  status_field: Status
  # Workflow keys mapped to the option names of status_field.
  columns:
    ready: Ready
    in_progress: In Progress
    review: In Review
    done: Done

rules:
  # Block "gh pr create" unless a card is claimed and in progress.
  require_claim_before_pr: true
  # Block the agent from stopping while its card has no actuals.
  require_actuals_before_stop: true
  # Required CI check (pinakas check-ci):
  # the pull request must close a card of the board ("Closes #N")...
  require_linked_issue: true
  # ...the card must have actuals...
  require_actuals_on_pr: true
  # ...and these labels (risk classes) need a human approval.
  approval_labels: ["risk:high"]
  # Authors skipped by the check, such as dependency bots.
  exempt_authors: ["dependabot[bot]", "renovate[bot]"]
`

const gitlabConfigTemplate = `# pinakas configuration — https://github.com/mmornati/pinakas
board:
  provider: gitlab
  # GitLab instance. The token comes from GITLAB_TOKEN (api scope).
  url: %s
  # Project path: the board, and the default project for bare issue numbers.
  repo: %s
  # Optional group path: the board then spans every project of the group.
  owner: %s
  # Workflow keys mapped to labels, one per column. Scoped labels keep
  # a single column per issue. Moving to done also closes the issue.
  columns:
    ready: "workflow::ready"
    in_progress: "workflow::in progress"
    review: "workflow::review"
    done: "workflow::done"

rules:
  # Block "gh pr create" / "glab mr create" unless a card is claimed and in progress.
  require_claim_before_pr: true
  # Block the agent from stopping while its card has no actuals.
  require_actuals_before_stop: true
`

func (app *App) initCommand() *cobra.Command {
	var owner, repo, provider, gitlabURL string
	var project int
	var copilot, codex, workflow bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write .pinakas.yml, and optionally the Copilot or Codex hooks and the GitHub workflow",
		Long: "Write .pinakas.yml in the current directory. --copilot adds .github/hooks/pinakas.json for GitHub Copilot, " +
			"--codex adds .codex/hooks.json and the pinakas skills in .agents/skills for OpenAI Codex, " +
			"--workflow adds .github/workflows/pinakas.yml for the board sync and the required check. Existing files are never overwritten.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			path := filepath.Join(app.Dir, config.FileName)
			if _, err := os.Stat(path); err == nil {
				if !copilot && !codex && !workflow {
					return fmt.Errorf("%s already exists", config.FileName)
				}
				fmt.Fprintf(out, "Kept the existing %s.\n", config.FileName)
			} else {
				content, hint := fmt.Sprintf(configTemplate, owner, project, repo), "pass --owner and --project"
				switch provider {
				case config.ProviderGitHub:
				case config.ProviderGitLab:
					content, hint = fmt.Sprintf(gitlabConfigTemplate, gitlabURL, repo, owner), "pass --repo group/project, or --owner group"
				default:
					return fmt.Errorf("unknown --provider %q (use github or gitlab)", provider)
				}
				if _, err := config.Parse([]byte(content)); err != nil {
					return fmt.Errorf("%w (%s)", err, hint)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(out, "Wrote %s. Check the column names against your board, then run `pinakas status`.\n", config.FileName)
			}
			type extra struct {
				on   bool
				path string
				body []byte
				next string
			}
			extras := []extra{
				{copilot, filepath.Join(".github", "hooks", "pinakas.json"), templates.CopilotHooks, "Copilot loads it at the next session; the cloud agent needs pinakas installed in copilot-setup-steps.yml."},
				{codex, filepath.Join(".codex", "hooks.json"), templates.CodexHooks, "Codex runs project hooks once you trust them: open `/hooks` in Codex and review them."},
			}
			if codex {
				skills, err := codexSkills()
				if err != nil {
					return err
				}
				for _, path := range slices.Sorted(maps.Keys(skills)) {
					body := skills[path]
					extras = append(extras, extra{true, path, body, "Codex discovers it from .agents/skills."})
				}
			}
			extras = append(extras, extra{workflow, filepath.Join(".github", "workflows", "pinakas.yml"), templates.Workflow, "Add the PINAKAS_TOKEN secret, then make \"pinakas / check\" a required check."})
			for _, e := range extras {
				if !e.on {
					continue
				}
				full := filepath.Join(app.Dir, e.path)
				if _, err := os.Stat(full); err == nil {
					fmt.Fprintf(out, "Kept the existing %s.\n", e.path)
					continue
				}
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(full, e.body, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(out, "Wrote %s. %s\n", e.path, e.next)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", config.ProviderGitHub, "board backend: github or gitlab")
	cmd.Flags().StringVar(&gitlabURL, "url", "https://gitlab.com", "GitLab instance (with --provider gitlab)")
	cmd.Flags().StringVar(&owner, "owner", "", "user or organization that owns the project (GitLab: optional group path)")
	cmd.Flags().IntVar(&project, "project", 0, "GitHub Project number")
	cmd.Flags().StringVar(&repo, "repo", "", "default repository (owner/name; GitLab: group/project)")
	cmd.Flags().BoolVar(&copilot, "copilot", false, "also write .github/hooks/pinakas.json for GitHub Copilot")
	cmd.Flags().BoolVar(&codex, "codex", false, "also write .codex/hooks.json and .agents/skills for OpenAI Codex")
	cmd.Flags().BoolVar(&workflow, "workflow", false, "also write .github/workflows/pinakas.yml")
	return cmd
}

// codexSkills maps .agents/skills/<name>/SKILL.md to the plugin skills, in
// a stable order.
func codexSkills() (map[string][]byte, error) {
	out := map[string][]byte{}
	err := fs.WalkDir(plugin.Skills, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(plugin.Skills, p)
		if err != nil {
			return err
		}
		out[filepath.Join(".agents", filepath.FromSlash(p))] = body
		return nil
	})
	return out, err
}

func (app *App) statusCommand() *cobra.Command {
	var asJSON, hook, mine bool
	var format string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the board and the card claimed in this working tree",
		Long:  "Show the board grouped by column. With --hook, print a short context block for a SessionStart hook and never fail.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			if !hook {
				return app.status(cmd, cmd.OutOrStdout(), asJSON, false, mine)
			}
			var buf strings.Builder
			err := app.status(cmd, &buf, asJSON, true, mine)
			if errors.Is(err, config.ErrNotFound) {
				return nil // not a pinakas repository: stay silent
			}
			if err != nil {
				buf.Reset()
				fmt.Fprintf(&buf, "pinakas: board unavailable (%v). Do not rely on notes about card status; retry `pinakas status`.\n", err)
			}
			return writeContext(cmd.OutOrStdout(), format, buf.String())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print cards as JSON")
	cmd.Flags().BoolVar(&hook, "hook", false, "print context for a SessionStart hook")
	cmd.Flags().BoolVar(&mine, "mine", false, "only cards assigned to me")
	addFormatFlag(cmd, &format)
	return cmd
}

func (app *App) status(cmd *cobra.Command, out io.Writer, asJSON, hook, mine bool) error {
	ws, err := app.workspace()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 20*time.Second)
	defer cancel()
	b, err := app.board(ws)
	if err != nil {
		return err
	}
	cards, err := b.Cards(ctx)
	if err != nil {
		return err
	}
	if mine {
		me, err := b.CurrentUser(ctx)
		if err != nil {
			return err
		}
		cards = slices.DeleteFunc(slices.Clone(cards), func(c board.Card) bool { return !slices.Contains(c.Assignees, me) })
	}
	st, err := ws.store.Load()
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"current": st.Current, "cards": cards})
	}
	if hook {
		writeHookContext(out, ws.cfg, cards, st)
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, col := range columnOrder(ws.cfg, cards) {
		group := byStatus(cards, col)
		if len(group) == 0 {
			continue
		}
		name := col
		if name == "" {
			name = "(no status)"
		}
		fmt.Fprintf(w, "%s (%d)\n", name, len(group))
		for _, c := range group {
			if len(c.Assignees) == 0 {
				fmt.Fprintf(w, "  %s\t%s\n", c.Ref(), c.Title)
			} else {
				fmt.Fprintf(w, "  %s\t%s\t%s\n", c.Ref(), c.Title, strings.Join(c.Assignees, ","))
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if c := st.Current; c != nil {
		if c.Open() {
			fmt.Fprintf(out, "\nClaimed here: %s#%d %s (since %s, actuals not recorded yet)\n", c.Repo, c.Number, c.Title, c.ClaimedAt.Local().Format("2006-01-02 15:04 MST"))
		} else {
			fmt.Fprintf(out, "\nLast card here: %s#%d %s (actuals recorded; nothing claimed)\n", c.Repo, c.Number, c.Title)
		}
	}
	return nil
}

func writeHookContext(out io.Writer, cfg config.Config, cards []board.Card, st state.State) {
	fmt.Fprintf(out, "pinakas: live board state (%s). This is the source of truth; ignore older notes about card status.\n", cfg.Name())
	if c := st.Current; c != nil {
		if c.Open() {
			fmt.Fprintf(out, "- Claimed in this working tree: %s#%d %s, since %s, actuals not recorded yet.\n", c.Repo, c.Number, c.Title, c.ClaimedAt.UTC().Format(time.RFC3339))
		} else {
			fmt.Fprintf(out, "- Last card in this working tree: %s#%d, actuals recorded.\n", c.Repo, c.Number)
		}
	} else {
		fmt.Fprintln(out, "- No card claimed in this working tree.")
	}
	for _, col := range columnOrder(cfg, cards) {
		if n := len(byStatus(cards, col)); n > 0 && col != "" {
			fmt.Fprintf(out, "- %s: %d\n", col, n)
		}
	}
	ready := byStatus(cards, cfg.Column(config.ColumnReady))
	if len(ready) > 0 {
		fmt.Fprintln(out, "Ready to claim:")
		for i, c := range ready {
			if i == 10 {
				fmt.Fprintf(out, "  … and %d more (`pinakas status`)\n", len(ready)-10)
				break
			}
			fmt.Fprintf(out, "  %s %s\n", c.Ref(), c.Title)
		}
	}
	fmt.Fprintln(out, "Workflow: `pinakas claim <issue>` before coding, open the PR only for the claimed card, `pinakas actuals <issue>` before stopping.")
}

// columnOrder lists the configured columns first, then any others found.
func columnOrder(cfg config.Config, cards []board.Card) []string {
	order := []string{
		cfg.Column(config.ColumnReady),
		cfg.Column(config.ColumnInProgress),
		cfg.Column(config.ColumnReview),
		cfg.Column(config.ColumnDone),
	}
	for _, c := range cards {
		if !slices.Contains(order, c.Status) {
			order = append(order, c.Status)
		}
	}
	return order
}

func byStatus(cards []board.Card, status string) []board.Card {
	var out []board.Card
	for _, c := range cards {
		if c.Status == status {
			out = append(out, c)
		}
	}
	return out
}

func (app *App) claimCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "claim <issue>",
		Short: "Assign a ready card to me and move it to in progress",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			st, err := ws.store.Load()
			if err != nil {
				return err
			}
			if st.Current.Open() && !force {
				return fmt.Errorf("%s#%d is still claimed without actuals: run `pinakas actuals %d` or `pinakas release` first (or --force)", st.Current.Repo, st.Current.Number, st.Current.Number)
			}
			ctx := cmd.Context()
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			card, err := app.findCard(ctx, ws, b, args[0])
			if err != nil {
				return err
			}
			me, err := b.CurrentUser(ctx)
			if err != nil {
				return err
			}
			ready := ws.cfg.Column(config.ColumnReady)
			if card.Status != ready && !force {
				return fmt.Errorf("%s is in %q, not %q: only ready cards can be claimed (or --force)", card.Ref(), card.Status, ready)
			}
			if others := slices.DeleteFunc(slices.Clone(card.Assignees), func(l string) bool { return l == me }); len(others) > 0 && !force {
				return fmt.Errorf("%s is already assigned to %s (or --force)", card.Ref(), strings.Join(others, ", "))
			}
			if !slices.Contains(card.Assignees, me) {
				if err := b.Assign(ctx, card, me); err != nil {
					return err
				}
			}
			if err := b.Move(ctx, card, ws.cfg.Column(config.ColumnInProgress)); err != nil {
				return err
			}
			previous := st.Current
			st.Current = &state.Claim{
				Repo: card.Repo, Number: card.Number, Title: card.Title, URL: card.URL,
				ItemID: card.ItemID, ClaimedBy: me, ClaimedAt: app.Now().UTC(),
			}
			if err := ws.store.Save(st); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Claimed %s %s: assigned to %s, moved to %q.\n", card.Ref(), card.Title, me, ws.cfg.Column(config.ColumnInProgress))
			if previous != nil && (previous.Repo != card.Repo || previous.Number != card.Number) {
				why := "its actuals are recorded"
				if previous.Open() {
					why = "it had no actuals (--force)"
				}
				fmt.Fprintf(out, "No longer tracking %s#%d here: %s.\n", previous.Repo, previous.Number, why)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "claim even if the card is not ready, assigned to someone else, or another claim is open")
	return cmd
}

func (app *App) moveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "move <issue> <column>",
		Short: "Move a card to a column (ready, in_progress, review, done, or a column name)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			card, err := app.findCard(cmd.Context(), ws, b, args[0])
			if err != nil {
				return err
			}
			column := ws.cfg.Column(args[1])
			if err := b.Move(cmd.Context(), card, column); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Moved %s to %q.\n", card.Ref(), column)
			if column != ws.cfg.Column(config.ColumnDone) {
				return nil
			}
			// A finished card leaves the working tree: once its actuals are
			// recorded, moving it to done ends the claim.
			st, err := ws.store.Load()
			if err != nil {
				return err
			}
			c := st.Current
			if c == nil || !strings.EqualFold(c.Repo, card.Repo) || c.Number != card.Number {
				return nil
			}
			if c.Open() {
				fmt.Fprintf(out, "Still claimed here: record `pinakas actuals %d` before stopping.\n", c.Number)
				return nil
			}
			st.Current = nil
			if err := ws.store.Save(st); err != nil {
				return err
			}
			fmt.Fprintf(out, "Claim on %s ended.\n", card.Ref())
			return nil
		},
	}
}

func (app *App) actualsCommand() *cobra.Command {
	var tokens int
	var firstPass, findings, agent, note string
	cmd := &cobra.Command{
		Use:   "actuals [issue]",
		Short: "Record what the claimed card really cost, as an issue comment",
		Long:  "Record duration (from the claim timestamp), tokens, first-pass success and review findings for the card claimed in this working tree.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			st, err := ws.store.Load()
			if err != nil {
				return err
			}
			c := st.Current
			if c == nil {
				return errors.New("no card is claimed in this working tree (`pinakas claim <issue>` first)")
			}
			if len(args) == 1 {
				repo, number, err := board.ParseRef(args[0])
				if err != nil {
					return err
				}
				if number != c.Number || (repo != "" && !strings.EqualFold(repo, c.Repo)) {
					return fmt.Errorf("%s is not the card claimed here (%s#%d)", args[0], c.Repo, c.Number)
				}
			}
			pass, err := actuals.ParseFirstPass(firstPass)
			if err != nil {
				return err
			}
			found, err := actuals.ParseFindings(findings)
			if err != nil {
				return err
			}
			now := app.Now().UTC()
			a := actuals.New(c.Repo, c.Number, c.ClaimedAt, now)
			if cmd.Flags().Changed("tokens") {
				if tokens < 0 {
					return errors.New("--tokens must be zero or more")
				}
				a.Tokens = &tokens
			}
			a.FirstPass, a.Findings, a.Agent, a.Note = pass, found, agent, note
			body, err := a.Comment()
			if err != nil {
				return err
			}
			b, err := app.board(ws)
			if err != nil {
				return err
			}
			card := board.Card{ItemID: c.ItemID, Repo: c.Repo, Number: c.Number}
			if err := b.Comment(cmd.Context(), card, body); err != nil {
				return err
			}
			c.ActualsAt = &now
			if err := ws.store.Save(st); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Recorded actuals on %s#%d: %s.\n", c.Repo, c.Number, a.DurationText())
			return nil
		},
	}
	cmd.Flags().IntVar(&tokens, "tokens", 0, "tokens used for the story")
	cmd.Flags().StringVar(&firstPass, "first-pass", "", "did the first review pass without changes (yes|no)")
	cmd.Flags().StringVar(&findings, "findings", "", "review findings by severity, e.g. major=1,minor=3")
	cmd.Flags().StringVar(&agent, "agent", os.Getenv("PINAKAS_AGENT"), "agent tool or model that did the work")
	cmd.Flags().StringVar(&note, "note", "", "free-form note appended to the comment")
	return cmd
}

func (app *App) releaseCommand() *cobra.Command {
	var to string
	var keepAssignee bool
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Drop the claim in this working tree without recording actuals, and unassign yourself",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := app.workspace()
			if err != nil {
				return err
			}
			st, err := ws.store.Load()
			if err != nil {
				return err
			}
			c := st.Current
			if c == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Nothing is claimed here.")
				return nil
			}
			done := []string{}
			unassign := c.ClaimedBy != "" && !keepAssignee
			if to != "" || unassign {
				b, err := app.board(ws)
				if err != nil {
					return err
				}
				card := board.Card{ItemID: c.ItemID, Repo: c.Repo, Number: c.Number}
				if unassign {
					if err := b.Unassign(cmd.Context(), card, c.ClaimedBy); err != nil {
						return err
					}
					done = append(done, "unassigned "+c.ClaimedBy)
				}
				if to != "" {
					column := ws.cfg.Column(to)
					if err := b.Move(cmd.Context(), card, column); err != nil {
						return err
					}
					done = append(done, fmt.Sprintf("moved to %q", column))
				}
			}
			st.Current = nil
			if err := ws.store.Save(st); err != nil {
				return err
			}
			msg := fmt.Sprintf("Released %s#%d", c.Repo, c.Number)
			if len(done) > 0 {
				msg += ": " + strings.Join(done, ", ")
			}
			fmt.Fprintln(cmd.OutOrStdout(), msg+".")
			return nil
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "also move the card to this column (e.g. ready)")
	cmd.Flags().BoolVar(&keepAssignee, "keep-assignee", false, "keep yourself assigned to the card")
	return cmd
}

func (app *App) checkPRCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "check-pr",
		Short: "Hook: block `gh pr create` unless the claimed card is in progress and assigned",
		Long:  "Reads a PreToolUse (Claude Code, Codex) or preToolUse (Copilot) payload on stdin. Claude Code format: exits 2 with the reason on stderr to block. Copilot and Codex formats: print a deny decision as JSON.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			in, err := hooks.ParseInput(cmd.InOrStdin())
			if err != nil {
				return app.hookWarning(err)
			}
			if !hooks.IsPRCreate(in.ToolInput.Command) {
				return nil
			}
			ws, err := app.workspace()
			if errors.Is(err, config.ErrNotFound) {
				return nil
			}
			if err != nil {
				return app.hookWarning(err)
			}
			st, err := ws.store.Load()
			if err != nil {
				return app.hookWarning(err)
			}
			var status *hooks.CardStatus
			if c := st.Current; c != nil {
				status = app.liveStatus(cmd.Context(), ws, c)
			}
			return verdict(cmd.OutOrStdout(), format, preToolUse, hooks.CheckPR(ws.cfg, in, st, status))
		},
	}
	addFormatFlag(cmd, &format)
	return cmd
}

// liveStatus asks the board for the claimed card; nil when unreachable.
func (app *App) liveStatus(ctx context.Context, ws *workspace, c *state.Claim) *hooks.CardStatus {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, err := app.board(ws)
	if err != nil {
		fmt.Fprintf(app.Stderr, "pinakas: board unreachable, checking the local claim only: %v\n", err)
		return nil
	}
	cards, err := b.Cards(ctx)
	if err != nil {
		fmt.Fprintf(app.Stderr, "pinakas: board unreachable, checking the local claim only: %v\n", err)
		return nil
	}
	card, err := board.Find(cards, c.Repo, c.Number)
	if err != nil {
		return &hooks.CardStatus{Status: "(not on the board)"}
	}
	return &hooks.CardStatus{Status: card.Status, Assignees: card.Assignees}
}

func (app *App) checkStopCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "check-stop",
		Short: "Hook: block stopping while the claimed card has no actuals",
		Long:  "Reads a Stop (Claude Code, Codex) or agentStop (Copilot) payload on stdin. Claude Code format: exits 2 with the reason on stderr to block. Copilot and Codex formats: print a block decision as JSON.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := checkFormat(format); err != nil {
				return err
			}
			in, err := hooks.ParseInput(cmd.InOrStdin())
			if err != nil {
				return app.hookWarning(err)
			}
			ws, err := app.workspace()
			if errors.Is(err, config.ErrNotFound) {
				return nil
			}
			if err != nil {
				return app.hookWarning(err)
			}
			st, err := ws.store.Load()
			if err != nil {
				return app.hookWarning(err)
			}
			return verdict(cmd.OutOrStdout(), format, stopEvent, hooks.CheckStop(ws.cfg, in, st))
		},
	}
	addFormatFlag(cmd, &format)
	return cmd
}

// hookWarning reports a problem without blocking the agent: a broken
// guardrail must never stop all work.
func (app *App) hookWarning(err error) error {
	fmt.Fprintln(app.Stderr, "pinakas: hook skipped:", err)
	return nil
}
