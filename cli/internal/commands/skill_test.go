package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"yingce/cli/internal/skill"
)

func TestSkillPrintsEmbeddedContent(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"skill"}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if out.String() != skill.Content() {
		t.Fatal("skill command did not print the embedded SKILL.md")
	}
}

func TestSkillJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"skill", "--json"}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, out.String())
	}
	if !payload.OK || payload.Data.Content != skill.Content() {
		t.Fatal("skill --json did not return the embedded content")
	}
}
