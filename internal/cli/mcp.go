package cli

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// The MCP server exposes the board to any MCP client. Each tool runs the
// matching CLI command in-process, so the rules (only ready cards can be
// claimed, one open claim per working tree, ...) are the same.

type claimIn struct {
	Issue string `json:"issue" jsonschema:"issue reference: 42, #42 or owner/name#42"`
	Force bool   `json:"force,omitempty" jsonschema:"claim even if the card is not ready, assigned to someone else, or another claim is open"`
}

type moveIn struct {
	Issue  string `json:"issue" jsonschema:"issue reference: 42, #42 or owner/name#42"`
	Column string `json:"column" jsonschema:"ready, in_progress, review, done, or a column name"`
}

type statusIn struct {
	Mine bool `json:"mine,omitempty" jsonschema:"only cards assigned to me"`
}

type actualsIn struct {
	Issue     string `json:"issue,omitempty" jsonschema:"the claimed card (optional check)"`
	Tokens    *int   `json:"tokens,omitempty" jsonschema:"tokens used for the story"`
	FirstPass string `json:"first_pass,omitempty" jsonschema:"did the first review pass without changes: yes or no"`
	Findings  string `json:"findings,omitempty" jsonschema:"review findings by severity, e.g. major=1,minor=3"`
	Agent     string `json:"agent,omitempty" jsonschema:"agent tool or model that did the work"`
	Note      string `json:"note,omitempty" jsonschema:"free-form note"`
}

type releaseIn struct {
	To           string `json:"to,omitempty" jsonschema:"also move the card to this column, e.g. ready"`
	KeepAssignee bool   `json:"keep_assignee,omitempty" jsonschema:"stay assigned to the card"`
}

type reportIn struct {
	Since    string   `json:"since,omitempty" jsonschema:"only stories delivered since: YYYY-MM-DD, RFC 3339 or 30d"`
	Status   []string `json:"status,omitempty" jsonschema:"columns to measure (default done)"`
	All      bool     `json:"all,omitempty" jsonschema:"measure every card on the board"`
	Measured bool     `json:"measured,omitempty" jsonschema:"only stories with actuals or timing"`
}

type defectIn struct {
	Story string `json:"story" jsonschema:"the delivered story that introduced the bug"`
	Bug   string `json:"bug" jsonschema:"the bug issue"`
	At    string `json:"at,omitempty" jsonschema:"when the bug was found: YYYY-MM-DD or RFC 3339 (default now)"`
	Note  string `json:"note,omitempty" jsonschema:"free-form note"`
}

// mcpServer builds the server. Tool calls are serialized: they share the
// claim state of the working tree.
func (app *App) mcpServer() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "pinakas", Title: "pinakas board", Version: version()}, &mcp.ServerOptions{
		Instructions: "The board is the source of truth. Pick only Ready cards, claim one before coding (one per working tree), " +
			"open the pull request only for the claimed card, and record actuals before stopping.",
	})
	var mu sync.Mutex
	run := func(args ...string) (*mcp.CallToolResult, any, error) {
		mu.Lock()
		defer mu.Unlock()
		var out, errOut bytes.Buffer
		sub := *app
		sub.Stdin, sub.Stdout, sub.Stderr = strings.NewReader(""), &out, &errOut
		code := Execute(&sub, args)
		text := strings.TrimSpace(out.String())
		if e := strings.TrimSpace(errOut.String()); e != "" {
			text = strings.TrimSpace(text + "\n" + e)
		}
		if text == "" {
			text = "ok"
		}
		return &mcp.CallToolResult{IsError: code != 0, Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	}
	flag := func(args []string, on bool, name string) []string {
		if on {
			return append(args, name)
		}
		return args
	}
	opt := func(args []string, name, value string) []string {
		if value != "" {
			return append(args, name, value)
		}
		return args
	}
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(s, &mcp.Tool{Name: "board_status", Description: "The board as JSON (cards with column and assignees) and the card claimed in this working tree.", Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, any, error) {
			return run(flag([]string{"status", "--json"}, in.Mine, "--mine")...)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "claim_card", Description: "Assign a Ready card to me, move it to In Progress and start the clock. Required before coding."},
		func(_ context.Context, _ *mcp.CallToolRequest, in claimIn) (*mcp.CallToolResult, any, error) {
			return run(flag([]string{"claim", in.Issue}, in.Force, "--force")...)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "move_card", Description: "Move a card to a column (ready, in_progress, review, done, or a column name)."},
		func(_ context.Context, _ *mcp.CallToolRequest, in moveIn) (*mcp.CallToolResult, any, error) {
			return run("move", in.Issue, in.Column)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "record_actuals", Description: "Record what the claimed card really cost (duration from the claim, tokens, first pass, findings) as an issue comment. Required before stopping."},
		func(_ context.Context, _ *mcp.CallToolRequest, in actualsIn) (*mcp.CallToolResult, any, error) {
			args := []string{"actuals"}
			if in.Issue != "" {
				args = append(args, in.Issue)
			}
			if in.Tokens != nil {
				args = append(args, "--tokens", strconv.Itoa(*in.Tokens))
			}
			args = opt(args, "--first-pass", in.FirstPass)
			args = opt(args, "--findings", in.Findings)
			args = opt(args, "--agent", in.Agent)
			args = opt(args, "--note", in.Note)
			return run(args...)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "release_card", Description: "Drop the claim without actuals (abandoned or blocked card) and unassign me."},
		func(_ context.Context, _ *mcp.CallToolRequest, in releaseIn) (*mcp.CallToolResult, any, error) {
			return run(flag(opt([]string{"release"}, "--to", in.To), in.KeepAssignee, "--keep-assignee")...)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "story_report", Description: "Per-story metrics and a summary as JSON: agent time, tokens, first pass, findings, lead time, defects within 30 days.", Annotations: readOnly},
		func(_ context.Context, _ *mcp.CallToolRequest, in reportIn) (*mcp.CallToolResult, any, error) {
			args := opt([]string{"report", "--format", "json"}, "--since", in.Since)
			if len(in.Status) > 0 {
				args = append(args, "--status", strings.Join(in.Status, ","))
			}
			return run(flag(flag(args, in.All, "--all"), in.Measured, "--measured")...)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "link_defect", Description: "Link a bug back to the delivered story that introduced it."},
		func(_ context.Context, _ *mcp.CallToolRequest, in defectIn) (*mcp.CallToolResult, any, error) {
			return run(opt(opt([]string{"defect", in.Story, in.Bug}, "--at", in.At), "--note", in.Note)...)
		})
	return s
}

func (app *App) mcpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the board to MCP clients over stdio",
		Long: "Run a Model Context Protocol server on stdin/stdout with the tools board_status, claim_card, move_card, " +
			"record_actuals, release_card, story_report and link_defect. Start it from the repository so that it finds .pinakas.yml.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.mcpServer().Run(cmd.Context(), &mcp.IOTransport{Reader: readCloser{cmd.InOrStdin()}, Writer: writeCloser{cmd.OutOrStdout()}})
		},
	}
}

type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

type writeCloser struct{ io.Writer }

func (writeCloser) Close() error { return nil }
