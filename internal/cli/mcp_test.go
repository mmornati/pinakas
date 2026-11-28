package cli

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpSession(t *testing.T, h *harness) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := h.app.mcpServer().Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestMCPServer(t *testing.T) {
	h := newHarness(t, true)
	cs := mcpSession(t, h)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if got := strings.Join(names, ","); got != "board_status,claim_card,link_defect,move_card,record_actuals,release_card,story_report" {
		t.Errorf("tools = %s", got)
	}

	text, isErr := call(t, cs, "board_status", nil)
	var status struct {
		Cards []map[string]any `json:"cards"`
	}
	if isErr || json.Unmarshal([]byte(text), &status) != nil || len(status.Cards) != 3 {
		t.Fatalf("board_status = %q (error %v)", text, isErr)
	}

	if text, isErr := call(t, cs, "claim_card", map[string]any{"issue": "3"}); !isErr || !strings.Contains(text, "only ready cards") {
		t.Errorf("claiming a done card must fail with the CLI reason: %q", text)
	}
	if text, isErr := call(t, cs, "claim_card", map[string]any{"issue": "#1"}); isErr || !strings.Contains(text, "Claimed acme/app#1") {
		t.Fatalf("claim_card = %q", text)
	}
	if text, isErr := call(t, cs, "record_actuals", map[string]any{"tokens": 900, "first_pass": "yes", "findings": "minor=1", "agent": "mcp"}); isErr || !strings.Contains(text, "Recorded actuals") {
		t.Fatalf("record_actuals = %q", text)
	}
	if text, isErr := call(t, cs, "move_card", map[string]any{"issue": "1", "column": "done"}); isErr || !strings.Contains(text, "Claim on acme/app#1 ended") {
		t.Errorf("move_card = %q", text)
	}
	if text, isErr := call(t, cs, "link_defect", map[string]any{"story": "1", "bug": "2", "note": "found via mcp"}); isErr || !strings.Contains(text, "Linked acme/app#2 to acme/app#1") {
		t.Errorf("link_defect = %q", text)
	}
	h.board.CardList[0].Status = "Done"
	text, isErr = call(t, cs, "story_report", map[string]any{"measured": true})
	var report struct {
		Summary struct {
			TotalTokens int `json:"total_tokens"`
			Defects     int `json:"defects_30d"`
		} `json:"summary"`
	}
	if isErr || json.Unmarshal([]byte(text), &report) != nil || report.Summary.TotalTokens != 900 || report.Summary.Defects != 1 {
		t.Errorf("story_report = %q", text)
	}
	if text, isErr := call(t, cs, "release_card", nil); isErr || !strings.Contains(text, "Nothing is claimed") {
		t.Errorf("release_card = %q", text)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "move_card", Arguments: map[string]any{"issue": 1}}); err == nil && !res.IsError {
		t.Error("a wrongly typed argument must be rejected")
	}
}
