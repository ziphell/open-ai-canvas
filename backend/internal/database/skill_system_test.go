package database

import (
	"path/filepath"
	"testing"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSkillContentPointerMigrationBackfillsAndDropsVersionTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "skill-pointer.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&legacySkillRow{}); err != nil {
		t.Fatalf("migrate legacy skills: %v", err)
	}
	if err := db.Exec("CREATE TABLE skill_versions (id text primary key)").Error; err != nil {
		t.Fatalf("create skill_versions: %v", err)
	}
	if err := db.Exec("CREATE TABLE skill_files (id text primary key)").Error; err != nil {
		t.Fatalf("create skill_files: %v", err)
	}
	if err := db.Create(&legacySkillRow{ID: "skill-1", Name: "技能", Description: "简介", CurrentVersionID: "content-1"}).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&legacySkillRow{ID: "skill-2", Name: "技能二", Description: "简介"}).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}

	if err := migrateSkillContentPointer(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if db.Migrator().HasTable("skill_versions") || db.Migrator().HasTable("skill_files") {
		t.Fatal("version tables were not dropped")
	}
	if db.Migrator().HasColumn("skills", "current_version_id") {
		t.Fatal("current_version_id column was not dropped")
	}
	var first, second model.Skill
	if err := db.First(&first, "id = ?", "skill-1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&second, "id = ?", "skill-2").Error; err != nil {
		t.Fatal(err)
	}
	if first.ContentID != "content-1" {
		t.Fatalf("content pointer = %q, want content-1", first.ContentID)
	}
	if second.ContentID != "skill-2" {
		t.Fatalf("empty content pointer = %q, want skill id", second.ContentID)
	}
}

// 旧库的 skills 表在 current_version_id 上带索引；SQLite 会因此拒绝 DROP COLUMN，
// 迁移 110 必须先把这些索引删掉，否则升级时后端会启动失败。
func TestSkillContentPointerMigrationDropsLegacyColumnIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "skill-pointer-index.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&legacySkillRow{}); err != nil {
		t.Fatalf("migrate legacy skills: %v", err)
	}
	if err := db.Exec("CREATE INDEX idx_skills_current_version_id ON skills(current_version_id)").Error; err != nil {
		t.Fatalf("create legacy index: %v", err)
	}
	if err := db.Exec("CREATE TABLE skill_versions (id text primary key)").Error; err != nil {
		t.Fatalf("create skill_versions: %v", err)
	}
	if err := db.Create(&legacySkillRow{ID: "skill-1", Name: "技能", CurrentVersionID: "content-1"}).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}

	if err := migrateSkillContentPointer(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if db.Migrator().HasColumn("skills", "current_version_id") {
		t.Fatal("current_version_id column was not dropped")
	}
	var remaining int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_skills_current_version_id'").Scan(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("legacy index idx_skills_current_version_id was not dropped")
	}
	var skill model.Skill
	if err := db.First(&skill, "id = ?", "skill-1").Error; err != nil {
		t.Fatal(err)
	}
	if skill.ContentID != "content-1" {
		t.Fatalf("content pointer = %q, want content-1", skill.ContentID)
	}
}

func TestSkillSystemPhase1BackfillsRelations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "skill-system.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Skill{}, &model.LegacyUserSkillState{}, &model.LegacySkillLibraryCategory{}); err != nil {
		t.Fatalf("migrate legacy: %v", err)
	}

	now := time.Now().UTC()
	public := model.Skill{
		ID: "skill-public", OwnerID: "admin", Name: "公开技能", Description: "公开简介",
		ContentID: "content-public", ContentHash: "hash-public", VersionLabel: "2", TotalBytes: 128,
		Status: 1, IsPrivate: false, SortWeight: 7, ShowcaseMediaJSON: "[]", ExtraInfo: "extra",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	owned := model.Skill{
		ID: "skill-owned", OwnerID: "user-1", Name: "我的技能", Description: "我的简介",
		ContentID: "content-owned", ContentHash: "hash-owned", VersionLabel: "1", TotalBytes: 64,
		Status: 1, IsPrivate: true, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	if err := db.Create(&public).Error; err != nil {
		t.Fatalf("create public: %v", err)
	}
	if err := db.Create(&owned).Error; err != nil {
		t.Fatalf("create owned: %v", err)
	}

	personal := model.LegacySkillLibraryCategory{ID: "cat-personal", Name: "我的分类", NormalizedName: "我的分类", Scope: "personal", OwnerID: "user-1", CreatedAt: now, UpdatedAt: now}
	platform := model.LegacySkillLibraryCategory{ID: "cat-platform", Name: "平台分类", NormalizedName: "平台分类", Scope: "platform", OwnerID: "__platform__", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&personal).Error; err != nil {
		t.Fatalf("create personal category: %v", err)
	}
	if err := db.Create(&platform).Error; err != nil {
		t.Fatalf("create platform category: %v", err)
	}

	state := model.LegacyUserSkillState{
		ID: "state-1", UserID: "user-1", SkillID: public.ID,
		Added: true, Liked: true, LibraryCategoryID: personal.ID,
		InstalledVersionID: public.ContentID, AutoUpdate: true,
		AgentMode: model.UserSkillAgentModeManual, AgentUseCount: 3,
		CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now,
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("create state: %v", err)
	}

	if err := migrateSkillSystemPhase1(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var listings []model.MarketSkillListing
	if err := db.Find(&listings).Error; err != nil {
		t.Fatalf("list listings: %v", err)
	}
	if len(listings) != 1 || listings[0].SkillID != public.ID || listings[0].Status != "published" || listings[0].SortWeight != 7 {
		t.Fatalf("unexpected listings: %+v", listings)
	}

	var installs []model.UserInstalledSkill
	if err := db.Order("source_skill_id asc").Find(&installs).Error; err != nil {
		t.Fatalf("list installs: %v", err)
	}
	if len(installs) != 2 {
		t.Fatalf("expected 2 installs, got %+v", installs)
	}
	bySkill := map[string]model.UserInstalledSkill{}
	for _, item := range installs {
		bySkill[item.ID] = item
	}
	installedPublic, ok := bySkill["skill-owned"]
	if !ok {
		t.Fatalf("owned install missing: %+v", installs)
	}
	if installedPublic.SourceSkillID != nil || installedPublic.FollowUpdates || installedPublic.Name != owned.Name {
		t.Fatalf("owned install not detached: %+v", installedPublic)
	}
	var installedCopy *model.UserInstalledSkill
	for index := range installs {
		if installs[index].SourceSkillID != nil && *installs[index].SourceSkillID == public.ID {
			installedCopy = &installs[index]
		}
	}
	if installedCopy == nil {
		t.Fatalf("public install missing: %+v", installs)
	}
	if !installedCopy.FollowUpdates || installedCopy.ContentID != public.ContentID || installedCopy.ContentHash != public.ContentHash {
		t.Fatalf("public install snapshot wrong: %+v", installedCopy)
	}
	if installedCopy.LibraryCategoryID == "" || installedCopy.LibraryCategoryID == personal.ID {
		t.Fatalf("personal category was not remapped: %+v", installedCopy)
	}
	if installedCopy.AgentMode != model.UserSkillAgentModeManual || installedCopy.AgentUseCount != 3 {
		t.Fatalf("agent state not copied: %+v", installedCopy)
	}

	var favorites []model.UserSkillFavorite
	if err := db.Find(&favorites).Error; err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].UserID != "user-1" || favorites[0].SkillID != public.ID {
		t.Fatalf("unexpected favorites: %+v", favorites)
	}

	var categories []model.UserSkillLibraryCategory
	if err := db.Find(&categories).Error; err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != 1 || categories[0].Name != personal.Name {
		t.Fatalf("unexpected categories: %+v", categories)
	}

	var contents []model.SkillContent
	if err := db.Find(&contents).Error; err != nil {
		t.Fatalf("list contents: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("expected 2 content placeholders, got %+v", contents)
	}

	// 重复执行应保持幂等。
	if err := migrateSkillSystemPhase1(db); err != nil {
		t.Fatalf("migrate again: %v", err)
	}
	var installCount int64
	if err := db.Model(&model.UserInstalledSkill{}).Count(&installCount).Error; err != nil {
		t.Fatalf("count installs: %v", err)
	}
	if installCount != 2 {
		t.Fatalf("migration is not idempotent, installs=%d", installCount)
	}
}
