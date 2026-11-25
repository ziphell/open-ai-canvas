package skills

import (
	"errors"
	"testing"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func createSkillTestUser(t *testing.T, db *gorm.DB, id string, role model.UserRole, status model.UserStatus) *model.User {
	t.Helper()
	user := &model.User{ID: id, Username: id, Role: role, Status: status}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func newUserSkillTestService(t *testing.T) (*Service, *gorm.DB, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+kernel.NewID()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&model.Skill{}, &model.SkillContent{}, &model.SkillContentFile{},
		&model.MarketSkillListing{}, &model.UserInstalledSkill{}, &model.UserSkillFavorite{},
		&model.UserSkillLibraryCategory{}, &model.User{}, &model.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	userA := createSkillTestUser(t, db, "user-a", model.UserRoleUser, model.UserStatusActive)
	userB := createSkillTestUser(t, db, "user-b", model.UserRoleUser, model.UserStatusActive)
	admin := createSkillTestUser(t, db, "admin-a", model.UserRoleAdmin, model.UserStatusActive)
	return New(repository.New(db), t.TempDir(), nil), db, userA, userB, admin
}

func TestPrivateUserSkillIsOwnerScoped(t *testing.T) {
	svc, _, userA, userB, _ := newUserSkillTestService(t)
	created, err := svc.CreateSkill(userA.ID, SkillMutationRequest{
		SkillName: "私有技能", Description: "只属于 A", Instruction: "# 私有技能\n\n第一版正文。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsOwner || !created.IsAdded || !created.IsPrivate || created.SkillID == "" {
		t.Fatalf("unexpected private item: %#v", created)
	}
	if _, err := svc.SkillDetail(userB.ID, created.SkillID); err == nil {
		t.Fatal("user B read user A private skill")
	}
	if _, err := svc.UpdateSkill(userB.ID, created.SkillID, SkillMutationRequest{SkillName: "hijack"}); err == nil {
		t.Fatal("user B updated user A private skill")
	}
	if err := svc.DeleteSkill(userB.ID, created.SkillID); err == nil {
		t.Fatal("user B deleted user A private skill")
	}
	if _, err := svc.SkillPackageFile(userB.ID, created.SkillID, "SKILL.md"); err == nil {
		t.Fatal("user B read user A private content")
	}
	file, err := svc.SkillPackageFile(userA.ID, created.SkillID, "SKILL.md")
	if err != nil || file.Content != "# 私有技能\n\n第一版正文。" {
		t.Fatalf("owner file = %#v, %v", file, err)
	}
	result, err := svc.EditSkillPackageFiles(userA.ID, created.SkillID, SkillPackageEditRequest{
		BaseVersionID: created.VersionID,
		Operations:    []SkillPackageFileOperation{{Op: SkillFileOperationWrite, Path: "SKILL.md", Content: stringPointer("# 私有技能\n\n第二版正文。\n")}},
	})
	if err != nil || !result.Changed || result.Skill.Instruction != "# 私有技能\n\n第二版正文。\n" {
		t.Fatalf("edit result = %#v, %v", result, err)
	}
}

func createMarketSkillFixture(t *testing.T, svc *Service, db *gorm.DB, contentID, body string) *model.Skill {
	t.Helper()
	archive, err := archiveFromMarkdown([]byte(body), "市场技能", "广场简介")
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		t.Fatal(err)
	}
	files := svc.contentFileRecords(contentID, archive.Files)
	if err := db.Create(&model.SkillContent{ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data}).Error; err != nil {
		t.Fatal(err)
	}
	if len(files) > 0 {
		if err := db.Create(&files).Error; err != nil {
			t.Fatal(err)
		}
	}
	skill := &model.Skill{
		ID: "market-skill", Name: "市场技能", Description: "广场简介", ContentID: contentID,
		ContentHash: archive.ContentHash, TotalBytes: archive.TotalBytes, FileCount: len(files),
		Status: 1, IsPrivate: false, Tag: "drama", SourceType: "markdown",
	}
	var existing model.Skill
	err = db.First(&existing, "id = ?", skill.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(skill).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.MarketSkillListing{SkillID: skill.ID, Status: "published", ShowcaseMediaJSON: "[]"}).Error; err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	return skill
}

func TestMarketplaceInstallFavoriteAndDetach(t *testing.T) {
	svc, db, userA, userB, _ := newUserSkillTestService(t)
	createMarketSkillFixture(t, svc, db, "market-content-1", "# 市场技能\n\n第一版市场内容。")

	list, err := svc.Skills(userA.ID, SkillListRequest{Scope: "public"})
	if err != nil || list.TotalCount != 1 || list.Skills[0].IsAdded {
		t.Fatalf("public list = %#v, %v", list, err)
	}
	added, err := svc.SetSkillAdded(userA.ID, "market-skill", true)
	if err != nil || !added.IsAdded {
		t.Fatalf("install = %#v, %v", added, err)
	}
	references, err := svc.AddedSkills(userA.ID)
	if err != nil || len(references) != 1 {
		t.Fatalf("added skills = %#v, %v", references, err)
	}
	instanceID := references[0].SkillID
	catalog, err := svc.AgentSkillCatalog(userA.ID)
	if err != nil || len(catalog) != 1 || !catalog[0].Installed {
		t.Fatalf("agent catalog = %#v, %v", catalog, err)
	}
	if _, err := svc.SetSkillLiked(userA.ID, "market-skill", true); err != nil {
		t.Fatal(err)
	}
	publicForB, err := svc.Skills(userB.ID, SkillListRequest{Scope: "public"})
	if err != nil || len(publicForB.Skills) != 1 || publicForB.Skills[0].AddedCount != 1 || publicForB.Skills[0].LikeCount != 1 {
		t.Fatalf("public metrics = %#v, %v", publicForB, err)
	}

	updated, err := svc.UpdateSkill(userA.ID, instanceID, SkillMutationRequest{SkillName: "市场技能（改）", Description: "本地简介"})
	if err != nil || updated.SkillName != "市场技能（改）" {
		t.Fatalf("rename instance = %#v, %v", updated, err)
	}
	instance, err := svc.repo.UserInstalledSkill(userA.ID, instanceID)
	if err != nil || instance == nil || instance.FollowUpdates || instance.FollowMetadata {
		t.Fatalf("edited instance should detach: %#v %v", instance, err)
	}

	// 广场更新新内容后，已编辑实例保持旧内容。
	createMarketSkillFixture(t, svc, db, "market-content-2", "# 市场技能\n\n第二版市场内容。")
	if err := db.Model(&model.Skill{}).Where("id = ?", "market-skill").Updates(map[string]any{"content_id": "market-content-2", "content_hash": "updated"}).Error; err != nil {
		t.Fatal(err)
	}
	detail, err := svc.SkillDetail(userA.ID, instanceID)
	if err != nil || detail.Instruction != "# 市场技能\n\n第一版市场内容。" {
		t.Fatalf("edited instance content changed: %#v %v", detail, err)
	}
	if _, err := svc.SetSkillAdded(userB.ID, "market-skill", true); err != nil {
		t.Fatal(err)
	}
	userBReferences, err := svc.AddedSkills(userB.ID)
	if err != nil || len(userBReferences) != 1 {
		t.Fatalf("user B added = %#v %v", userBReferences, err)
	}
	userBDetail, err := svc.SkillDetail(userB.ID, userBReferences[0].SkillID)
	if err != nil || userBDetail.Instruction != "# 市场技能\n\n第二版市场内容。" {
		t.Fatalf("new install should use latest content: %#v %v", userBDetail, err)
	}
}

func TestMarketplaceHiddenPreventsInstallAndAdminRequiresRole(t *testing.T) {
	svc, db, userA, _, admin := newUserSkillTestService(t)
	createMarketSkillFixture(t, svc, db, "market-content-1", "# 市场技能\n\n内容。")
	if _, err := svc.AdminSkills(&model.User{ID: userA.ID, Role: model.UserRoleUser}, AdminSkillListRequest{}); err == nil {
		t.Fatal("ordinary user accessed admin skill list")
	}
	if _, err := svc.AdminSetSkillEnabled(admin, "market-skill", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSkillAdded(userA.ID, "market-skill", true); err == nil {
		t.Fatal("hidden market skill was installed")
	}
	public, err := svc.Skills(userA.ID, SkillListRequest{Scope: "public"})
	if err != nil || public.TotalCount != 0 {
		t.Fatalf("hidden listing still public: %#v %v", public, err)
	}
}

func TestAgentMarketCatalogExcludesInstalledSkills(t *testing.T) {
	svc, db, userA, userB, _ := newUserSkillTestService(t)
	createMarketSkillFixture(t, svc, db, "market-content-1", "# 市场技能\n\n内容。")
	market, err := svc.MarketSkillCatalog(userB.ID)
	if err != nil || len(market) != 1 || market[0].Installed {
		t.Fatalf("market catalog = %#v %v", market, err)
	}
	if _, err := svc.SetSkillAdded(userB.ID, "market-skill", true); err != nil {
		t.Fatal(err)
	}
	market, err = svc.MarketSkillCatalog(userB.ID)
	if err != nil || len(market) != 0 {
		t.Fatalf("installed skill still in market catalog: %#v %v", market, err)
	}
	catalog, err := svc.AgentSkillCatalog(userB.ID)
	if err != nil || len(catalog) != 1 {
		t.Fatalf("user catalog = %#v %v", catalog, err)
	}
	if other, err := svc.AgentSkillCatalog(userA.ID); err != nil || len(other) != 0 {
		t.Fatalf("other user catalog leaked: %#v %v", other, err)
	}
}

func TestInstalledSkillTracksMarketUpdatesUntilEdited(t *testing.T) {
	svc, db, userA, _, _ := newUserSkillTestService(t)
	createMarketSkillFixture(t, svc, db, "market-content-1", "# 市场技能\n\n第一版市场内容。")
	if _, err := svc.SetSkillAdded(userA.ID, "market-skill", true); err != nil {
		t.Fatal(err)
	}
	references, err := svc.AddedSkills(userA.ID)
	if err != nil || len(references) != 1 {
		t.Fatalf("added skills = %#v %v", references, err)
	}
	instanceID := references[0].SkillID
	if detail, err := svc.SkillDetail(userA.ID, instanceID); err != nil || detail.HasUpdate || !detail.FollowUpdates {
		t.Fatalf("fresh install should follow without update: %#v %v", detail, err)
	}

	createMarketSkillFixture(t, svc, db, "market-content-2", "# 市场技能\n\n第二版市场内容。")
	var nextContent model.SkillContent
	if err := db.First(&nextContent, "id = ?", "market-content-2").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Skill{}).Where("id = ?", "market-skill").Updates(map[string]any{
		"content_id": "market-content-2", "content_hash": nextContent.SHA256,
	}).Error; err != nil {
		t.Fatal(err)
	}
	beforeUpdate, err := svc.SkillDetail(userA.ID, instanceID)
	if err != nil || !beforeUpdate.HasUpdate {
		t.Fatalf("followed install should report update: %#v %v", beforeUpdate, err)
	}
	updated, err := svc.UpdateUserSkillFromMarket(userA.ID, instanceID)
	if err != nil || updated.Instruction != "# 市场技能\n\n第二版市场内容。" || updated.HasUpdate || !updated.FollowUpdates {
		t.Fatalf("update from market = %#v %v", updated, err)
	}

	if _, err := svc.EditSkillPackageFiles(userA.ID, instanceID, SkillPackageEditRequest{
		BaseVersionID: updated.VersionID,
		Operations:    []SkillPackageFileOperation{{Op: SkillFileOperationWrite, Path: "SKILL.md", Content: stringPointer("# 市场技能\n\n我的本地版本。\n")}},
	}); err != nil {
		t.Fatal(err)
	}
	edited, err := svc.SkillDetail(userA.ID, instanceID)
	if err != nil || edited.FollowUpdates || edited.HasUpdate {
		t.Fatalf("edited install should stop following: %#v %v", edited, err)
	}
	if _, err := svc.UpdateUserSkillFromMarket(userA.ID, instanceID); err == nil {
		t.Fatal("edited install accepted a market update")
	}
}

func TestSkillAdministratorAuthorizationUsesPersistedAccount(t *testing.T) {
	_, db, userA, _, admin := newUserSkillTestService(t)
	createSkillTestUser(t, db, "disabled-admin", model.UserRoleAdmin, model.UserStatusDisabled)
	for _, test := range []struct {
		id     string
		admin  bool
		status int
	}{
		{"", false, 401},
		{"missing-user", false, 401},
		{userA.ID, false, 0},
		{"disabled-admin", false, 403},
		{admin.ID, true, 0},
	} {
		isAdmin, err := svcIsAdminHelper(db, test.id)
		if isAdmin != test.admin || appErrorStatus(err) != test.status || (test.status == 0 && err != nil) {
			t.Fatalf("IsAdministrator(%q) = %v %v", test.id, isAdmin, err)
		}
	}
	if isAdmin, err := svcIsAdminHelper(db, admin.ID); err != nil || !isAdmin {
		t.Fatalf("persisted admin check = %v %v", isAdmin, err)
	}
}

func svcIsAdminHelper(db *gorm.DB, id string) (bool, error) {
	return New(repository.New(db), "", nil).IsAdministrator(id)
}
