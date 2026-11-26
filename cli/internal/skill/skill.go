package skill

import _ "embed"

//go:embed SKILL.md
var content string

func Content() string { return content }
