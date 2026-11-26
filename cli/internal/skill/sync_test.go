package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// 分发目录与二进制内嵌副本必须一致，否则 npx skills add 安装的内容会和 CLI 输出漂移。
func TestDistributedSkillMatchesEmbedded(t *testing.T) {
	path := filepath.Join("..", "..", "..", "skills", "yingce-cli", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read distributed skill: %v", err)
	}
	if string(data) != Content() {
		t.Fatal("skills/yingce-cli/SKILL.md 与 cli/internal/skill/SKILL.md 不一致，请同步后再提交")
	}
}
