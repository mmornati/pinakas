// Package metrics measures delivered stories from the markers pinakas
// leaves on their issues: actuals, timing and defects.
package metrics

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DefectMarker opens the machine-readable part of a defect comment.
const DefectMarker = "<!-- pinakas:defect "

// Defect links a bug back to the story that introduced it.
type Defect struct {
	Version      int       `json:"v"`
	Repo         string    `json:"repo"`
	Number       int       `json:"number"`
	DefectRepo   string    `json:"defect_repo"`
	DefectNumber int       `json:"defect_number"`
	FoundAt      time.Time `json:"found_at"`
	Note         string    `json:"note,omitempty"`
}

// Ref returns owner/name#number of the bug.
func (d Defect) Ref() string { return fmt.Sprintf("%s#%d", d.DefectRepo, d.DefectNumber) }

// Comment renders the comment posted on the story issue.
func (d Defect) Comment() (string, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s -->\n", DefectMarker, raw)
	fmt.Fprintf(&b, "### pinakas defect\n\n%s was found on %s and traced back to this story.\n", d.Ref(), d.FoundAt.UTC().Format(time.DateOnly))
	if d.Note != "" {
		fmt.Fprintf(&b, "\n%s\n", d.Note)
	}
	return b.String(), nil
}

// ParseDefect extracts a defect from a comment body written by Comment.
func ParseDefect(body string) (Defect, bool) {
	_, rest, ok := strings.Cut(body, DefectMarker)
	if !ok {
		return Defect{}, false
	}
	payload, _, ok := strings.Cut(rest, " -->")
	if !ok {
		return Defect{}, false
	}
	var d Defect
	if json.Unmarshal([]byte(payload), &d) != nil {
		return Defect{}, false
	}
	return d, true
}
