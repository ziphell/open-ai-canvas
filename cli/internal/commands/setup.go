package commands

import (
	"io"
	"os"
	"path/filepath"

	"yingce/cli/internal/skill"
)

func runSetup(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var agent, dir string
	var write bool
	fs := newFlagSet("setup", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&agent, "agent", "", "claude-code / cursor / codex / generic")
	fs.StringVar(&dir, "dir", "", "generic 模式下的 skill 目录")
	fs.BoolVar(&write, "write", false, "实际写入；缺省只打印路径")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	target, err := skillPath(agent, dir)
	if err != nil {
		return app.fail(err)
	}
	path := filepath.Join(target, "SKILL.md")
	if write {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return app.fail(err)
		}
		if err := os.WriteFile(path, []byte(skill.Content()), 0o644); err != nil {
			return app.fail(err)
		}
	}
	return app.fail(app.emit(marshal(map[string]any{
		"agent":   agent,
		"path":    path,
		"written": write,
	})))
}

func skillPath(agent, dir string) (string, error) {
	if agent == "" {
		return "", usagef("需要 --agent claude-code|cursor|codex|generic")
	}
	if agent == "generic" {
		if dir == "" {
			return "", usagef("generic 模式需要 --dir")
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch agent {
	case "claude-code":
		return filepath.Join(home, ".claude", "skills", "yingce-cli"), nil
	case "cursor":
		return filepath.Join(home, ".cursor", "skills", "yingce-cli"), nil
	case "codex":
		return filepath.Join(home, ".codex", "skills", "yingce-cli"), nil
	default:
		return "", usagef("未知 agent：%s", agent)
	}
}
