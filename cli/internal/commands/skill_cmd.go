package commands

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"yingce/cli/internal/skill"
)

// runSkill 把内置的 SKILL.md 原样输出到 stdout，由调用方决定如何保存或注入。
// CLI 不写任何第三方 Agent 目录。
func runSkill(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := flag.NewFlagSet("skill", flag.ContinueOnError)
	fs.SetOutput(errw)
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	if g.JSON {
		payload, err := json.Marshal(map[string]any{"ok": true, "data": map[string]any{"content": skill.Content()}})
		if err != nil {
			fmt.Fprintf(errw, "error: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, string(payload))
		return 0
	}
	fmt.Fprint(out, skill.Content())
	return 0
}
