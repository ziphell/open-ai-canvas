package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

// Only skill-positive fixtures opt in; creationTestService remains an ordinary user.
func skillAdministratorFixture(t *testing.T, db *gorm.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		result := db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{"role": model.UserRoleAdmin, "status": model.UserStatusActive})
		if result.Error != nil {
			t.Fatal(result.Error)
		}
		if result.RowsAffected == 0 {
			if err := db.Create(&model.User{ID: id, Username: "skill-admin-" + id, Role: model.UserRoleAdmin, Status: model.UserStatusActive}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
}

func assertSkillRuntimeStatus(t *testing.T, err error, status int) {
	t.Helper()
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Status != status {
		t.Fatalf("error = %v, want HTTP %d", err, status)
	}
}

func skillAuthorizationRequest() CloudAgentRequest {
	req := agentTestRequest()
	req.CanvasID, req.ContextScope = "", nil
	return req
}

func TestCloudAgentSkillCapabilitiesIncludeToolsForOrdinaryUsers(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	admin, names, err := s.CloudAgentSkillCapabilities("user")
	if err != nil || admin {
		t.Fatalf("capabilities = %v %v", admin, err)
	}
	found := false
	for _, name := range names {
		if cloudAgentIsSkillTool(name) {
			found = true
		}
	}
	if !found {
		t.Fatalf("ordinary user capabilities omit skill tools: %v", names)
	}
}

func TestCloudAgentSkillPoolUsesOwnedSkillInstances(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	skill, err := s.CreateSkill("user", SkillMutationRequest{
		SkillName: "私有技能", Description: "用户自己的技能", Instruction: "# 私有技能\n\nsecret-body",
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := s.cloudAgentBuildSkillPool("user", "hello", nil, defaultCloudAgentContextBudget())
	if err != nil {
		t.Fatal(err)
	}
	if pool.Versions[skill.SkillID] == "" {
		t.Fatalf("owned skill missing from pool: %#v", pool.Versions)
	}
	if !strings.Contains(pool.Catalog.Text, skill.SkillName) {
		t.Fatalf("catalog missing skill: %q", pool.Catalog.Text)
	}
	pinned, err := s.cloudAgentBuildSkillPool("user", "", []string{skill.SkillID}, defaultCloudAgentContextBudget())
	if err != nil || len(pinned.Pinned) != 1 {
		t.Fatalf("pinned pool = %#v %v", pinned.Pinned, err)
	}
}

func TestCloudAgentSkillReadUsesUserInstance(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	skill, err := s.CreateSkill("user", SkillMutationRequest{
		SkillName: "私有技能", Description: "用户自己的技能", Instruction: "# 私有技能\n\nsecret-body",
	})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := s.cloudAgentBuildSkillPool("user", "hello", nil, defaultCloudAgentContextBudget())
	if err != nil {
		t.Fatal(err)
	}
	state := &cloudAgentRuntime{SkillVersions: pool.Versions}
	result, err := cloudAgentReadTool(s.repo, "user", state, skillFeedbackCall(skill.SkillID, "SKILL.md"), s)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if !strings.Contains(string(encoded), "secret-body") {
		t.Fatalf("skill body not read: %s", encoded)
	}
}

func TestCloudAgentSkillReadRejectsAnotherUsersInstance(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.Create(&model.User{ID: "author", Username: "author", Role: model.UserRoleUser, Status: model.UserStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	skill, err := s.CreateSkill("author", SkillMutationRequest{
		SkillName: "作者技能", Description: "只给作者", Instruction: "# 作者技能\n\nsecret-body",
	})
	if err != nil {
		t.Fatal(err)
	}
	state := &cloudAgentRuntime{SkillVersions: map[string]string{skill.SkillID: skill.VersionID}}
	if _, err := cloudAgentReadTool(s.repo, "user", state, skillFeedbackCall(skill.SkillID, "SKILL.md"), s); err == nil {
		t.Fatal("user read another user's skill instance")
	}
}

func TestCloudAgentSkillExecutionAllowsSkillContextForUsers(t *testing.T) {
	s, _, _, _ := creationTestService(t)
	state := &cloudAgentRuntime{Skills: []cloudAgentSkill{{ID: "old", Instruction: "secret-body"}}}
	if err := s.cloudAgentCheckSkillExecution("user", state, nil, ""); err != nil {
		t.Fatalf("skill context rejected for ordinary user: %v", err)
	}
}
