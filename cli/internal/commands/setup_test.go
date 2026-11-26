package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"yingce/cli/internal/config"
	"yingce/cli/internal/skill"
)

func TestSetupWriteAndUpdate(t *testing.T) {
	configDir := t.TempDir()
	skillDir := t.TempDir()
	var out, errOut bytes.Buffer

	code := Run([]string{"setup", "--agent", "generic", "--dir", skillDir, "--write", "--json", "--config-dir", configDir}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("write exit %d: %s", code, errOut.String())
	}
	path := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != skill.Content() {
		t.Fatal("setup did not write the embedded skill")
	}

	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	code = Run([]string{"setup", "--update", "--json", "--config-dir", configDir}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("update exit %d: %s", code, errOut.String())
	}
	content, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != skill.Content() {
		t.Fatal("setup --update did not refresh the installed skill")
	}
	if !strings.Contains(out.String(), "SKILL.md") {
		t.Fatalf("unexpected update output %s", out.String())
	}

	cfg, _, err := config.Load(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SkillTargets) != 1 || cfg.SkillTargets[0].Path != skillDir || cfg.SkillTargets[0].Agent != "generic" {
		t.Fatalf("unexpected skill targets %+v", cfg.SkillTargets)
	}
}

func TestSetupUpdateWithoutTargets(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run([]string{"setup", "--update", "--config-dir", t.TempDir()}, &out, &errOut, strings.NewReader(""))
	if code != 2 {
		t.Fatalf("exit %d want 2", code)
	}
	if !strings.Contains(errOut.String(), "还没有安装记录") {
		t.Fatalf("unexpected error %s", errOut.String())
	}
}
