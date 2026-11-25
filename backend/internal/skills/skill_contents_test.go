package skills

import (
	"os"
	"path/filepath"
	"testing"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureSkillContentsImportsArchiveAndLegacyInstruction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+kernel.NewID()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.Skill{}, &model.SkillContent{}, &model.SkillContentFile{}, &model.UserInstalledSkill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dataDir := t.TempDir()
	archive, err := archiveFromMarkdown([]byte("# 测试技能\n\n这是测试简介"), "测试技能", "这是测试简介")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	zipData, err := encodeSkillArchive(archive.Files)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	packageKey := "skill-1/version-1.zip"
	absolute := filepath.Join(dataDir, "skill-packages", filepath.FromSlash(packageKey))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absolute, zipData, 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	skill := model.Skill{
		ID: "skill-1", Name: "测试技能", Description: "这是测试简介",
		ContentID: "version-1", ContentHash: archive.ContentHash, TotalBytes: archive.TotalBytes, Status: 1,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := db.Create(&model.SkillContent{ID: "version-1", Format: "zip", Size: archive.TotalBytes, SHA256: archive.ContentHash}).Error; err != nil {
		t.Fatalf("create content placeholder: %v", err)
	}

	legacy := model.Skill{ID: "legacy-1", Name: "传统技能", Description: "传统简介", ContentID: "legacy-1", Instruction: "# 传统技能\n\n传统简介", Status: 1}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy skill: %v", err)
	}
	if err := db.Create(&model.SkillContent{ID: "legacy-1", Format: "zip"}).Error; err != nil {
		t.Fatalf("create legacy content: %v", err)
	}

	svc := New(repository.New(db), dataDir, nil)
	if err := svc.EnsureSkillContents(); err != nil {
		t.Fatalf("ensure contents: %v", err)
	}

	for _, contentID := range []string{"version-1", "legacy-1"} {
		var content model.SkillContent
		if err := db.First(&content, "id = ?", contentID).Error; err != nil {
			t.Fatalf("load content %s: %v", contentID, err)
		}
		if len(content.Data) == 0 {
			t.Fatalf("content %s has no data", contentID)
		}
		var files []model.SkillContentFile
		if err := db.Where("content_id = ?", contentID).Find(&files).Error; err != nil {
			t.Fatalf("list files %s: %v", contentID, err)
		}
		if len(files) != 1 || files[0].Path != "SKILL.md" || files[0].Size == 0 {
			t.Fatalf("unexpected files for %s: %+v", contentID, files)
		}
	}

	snapshot, ok := svc.contentSnapshot(&skill)
	if !ok || snapshot.content == nil {
		t.Fatalf("content snapshot not used after import")
	}
	entry, err := svc.readSkillPackageSnapshotEntry(snapshot, "SKILL.md")
	if err != nil || len(entry) == 0 {
		t.Fatalf("read content entry: %v len=%d", err, len(entry))
	}
	mismatched := skill
	mismatched.ContentHash = "different"
	if _, ok := svc.contentSnapshot(&mismatched); ok {
		t.Fatalf("content snapshot should fall back when hash mismatches")
	}

	// 已导入内容不应重复处理。
	if err := svc.EnsureSkillContents(); err != nil {
		t.Fatalf("ensure contents again: %v", err)
	}
	var fileCount int64
	if err := db.Model(&model.SkillContentFile{}).Count(&fileCount).Error; err != nil {
		t.Fatalf("count files: %v", err)
	}
	if fileCount != 2 {
		t.Fatalf("expected 2 indexed files, got %d", fileCount)
	}
}
