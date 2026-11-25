package database

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

// 迁移 109：技能关系表已由 user_installed_skills / user_skill_favorites /
// user_skill_library_categories 取代，删除不再读写的旧表。
// 数据在迁移 106 已回填；删除后不可恢复，升级前请先备份。
func migrateSkillLegacyRelationCleanup(tx *gorm.DB) error {
	for _, table := range model.LegacySkillTables() {
		if !tx.Migrator().HasTable(table) {
			continue
		}
		if err := tx.Migrator().DropTable(table); err != nil {
			return err
		}
	}
	return nil
}
