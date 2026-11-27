package board

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	ok := map[string][2]any{
		"42":         {"", 42},
		"#42":        {"", 42},
		"acme/app#7": {"acme/app", 7},
		"acme/app#8": {"acme/app", 8},
	}
	for in, want := range ok {
		repo, n, err := ParseRef(in)
		if err != nil || repo != want[0] || n != want[1] {
			t.Errorf("ParseRef(%q) = %q, %d, %v", in, repo, n, err)
		}
	}
	for _, bad := range []string{"", "x", "0", "-3", "4x", "app#3", "a/b/c#3"} {
		if _, _, err := ParseRef(bad); err == nil {
			t.Errorf("ParseRef(%q) should fail", bad)
		}
	}
}

func TestFind(t *testing.T) {
	cards := []Card{
		{Repo: "acme/app", Number: 1},
		{Repo: "acme/api", Number: 1},
		{Repo: "acme/app", Number: 2},
	}
	if c, err := Find(cards, "", 2); err != nil || c.Repo != "acme/app" {
		t.Errorf("Find unique = %+v, %v", c, err)
	}
	if c, err := Find(cards, "ACME/api", 1); err != nil || c.Repo != "acme/api" {
		t.Errorf("Find with repo = %+v, %v", c, err)
	}
	if _, err := Find(cards, "", 1); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("Find ambiguous err = %v", err)
	}
	if _, err := Find(cards, "", 9); !errors.Is(err, ErrCardNotFound) {
		t.Errorf("Find missing err = %v", err)
	}
}
