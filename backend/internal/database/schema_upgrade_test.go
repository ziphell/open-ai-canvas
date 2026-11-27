package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestMigrateSchemaUpgradesFromSnapshot 用 schema=105 的真实数据库结构快照跑一遍升级迁移。
// 真实老库在 skills.current_version_id 上带索引 idx_skills_current_version_id，
// 迁移 110 必须先删索引再删列，否则升级时后端启动即失败（生产曾因此 crash-loop）。
// 快照只含结构与迁移记录，不含业务数据。
func TestMigrateSchemaUpgradesFromSnapshot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "schema-v105.sql"))
	if err != nil {
		t.Fatalf("读取结构快照: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开数据库: %v", err)
	}
	if err := execSQLScript(db, string(raw)); err != nil {
		t.Fatalf("载入结构快照: %v", err)
	}

	var from int64
	if err := db.Raw("SELECT max(version) FROM schema_migrations").Scan(&from).Error; err != nil {
		t.Fatalf("读取快照版本: %v", err)
	}
	if from != 105 {
		t.Fatalf("快照版本 = %d，期望 105", from)
	}

	if err := MigrateSchema(db); err != nil {
		t.Fatalf("升级迁移失败: %v", err)
	}

	status, err := ReadSchemaStatus(db)
	if err != nil {
		t.Fatalf("读取结构状态: %v", err)
	}
	if status.Current != CurrentSchemaVersion || !status.Ready {
		t.Fatalf("升级后结构版本 = %d ready=%v，期望 %d ready=true", status.Current, status.Ready, CurrentSchemaVersion)
	}
	if db.Migrator().HasColumn("skills", "current_version_id") {
		t.Fatal("skills.current_version_id 未删除")
	}
	var remaining int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_skills_current_version_id'").Scan(&remaining).Error; err != nil {
		t.Fatalf("查询遗留索引: %v", err)
	}
	if remaining != 0 {
		t.Fatal("遗留索引 idx_skills_current_version_id 未删除")
	}
	if db.Migrator().HasTable("skill_versions") || db.Migrator().HasTable("skill_files") {
		t.Fatal("被内容快照取代的 version 表未删除")
	}
}

// execSQLScript 逐条执行以分号结尾的 SQL 脚本；表结构与索引的 CREATE 语句可能跨多行。
func execSQLScript(db *gorm.DB, script string) error {
	pending := make([]string, 0, 8)
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || (strings.HasPrefix(trimmed, "--") && len(pending) == 0) {
			continue
		}
		pending = append(pending, line)
		if !strings.HasSuffix(trimmed, ";") {
			continue
		}
		stmt := strings.TrimSpace(strings.Join(pending, "\n"))
		pending = pending[:0]
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			head := stmt
			if len(head) > 80 {
				head = head[:80]
			}
			return fmt.Errorf("%s：%w", head, err)
		}
	}
	return nil
}
