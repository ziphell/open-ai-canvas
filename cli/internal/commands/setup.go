package commands

import (
	"io"
	"os"
	"path/filepath"

	"yingce/cli/internal/config"
	"yingce/cli/internal/skill"
)

func runSetup(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var agent, dir string
	var write, update bool
	fs := newFlagSet("setup", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&agent, "agent", "", "claude-code / cursor / codex / generic")
	fs.StringVar(&dir, "dir", "", "generic 模式下的 skill 目录")
	fs.BoolVar(&write, "write", false, "首次安装；缺省只打印路径")
	fs.BoolVar(&update, "update", false, "用当前 CLI 内置的 SKILL.md 覆盖已安装的目标")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if update {
		return app.fail(app.updateSkills(agent, dir))
	}
	target, err := skillPath(agent, dir)
	if err != nil {
		return app.fail(err)
	}
	path := filepath.Join(target, "SKILL.md")
	if write {
		if err := writeSkill(target); err != nil {
			return app.fail(err)
		}
		if err := app.rememberSkillTarget(agent, target); err != nil {
			return app.fail(err)
		}
	}
	return app.fail(app.emit(marshal(map[string]any{
		"agent":   agent,
		"path":    path,
		"written": write,
	})))
}

// updateSkills 用当前二进制内置的 SKILL.md 刷新已安装目录；未记录目标时可显式指定 agent。
func (a *App) updateSkills(agent, dir string) error {
	targets := a.Config.SkillTargets
	if agent != "" {
		target, err := skillPath(agent, dir)
		if err != nil {
			return err
		}
		targets = []config.SkillTarget{{Agent: agent, Path: target}}
	}
	if len(targets) == 0 {
		return usagef("还没有安装记录：先运行 yingce setup --agent <name> --write")
	}
	updated := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		if err := writeSkill(target.Path); err != nil {
			return err
		}
		if err := a.rememberSkillTarget(target.Agent, target.Path); err != nil {
			return err
		}
		updated = append(updated, map[string]any{
			"agent": target.Agent,
			"path":  filepath.Join(target.Path, "SKILL.md"),
		})
	}
	return a.emit(marshal(map[string]any{"updated": updated}))
}

func writeSkill(target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(skill.Content()), 0o644)
}

func (a *App) rememberSkillTarget(agent, target string) error {
	for index := range a.Config.SkillTargets {
		if a.Config.SkillTargets[index].Path == target {
			a.Config.SkillTargets[index].Agent = agent
			return config.SaveConfig(a.Dir, a.Config)
		}
	}
	a.Config.SkillTargets = append(a.Config.SkillTargets, config.SkillTarget{Agent: agent, Path: target})
	return config.SaveConfig(a.Dir, a.Config)
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
