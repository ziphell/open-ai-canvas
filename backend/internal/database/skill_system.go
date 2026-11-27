package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Phase 1 只新增表并从旧表回填关系数据，不删除旧表、不切换读路径。
// 内容 BLOB 由后续启动导入任务写入 skill_contents；这里先建立内容元数据占位。
func migrateSkillSystemPhase1(tx *gorm.DB) error {
	if err := tx.AutoMigrate(
		&model.SkillContent{},
		&model.MarketSkillListing{},
		&model.UserInstalledSkill{},
		&model.UserSkillFavorite{},
		&model.UserSkillLibraryCategory{},
	); err != nil {
		return err
	}

	// 旧迁移测试只建到指定版本，可能没有 skills 表；新表已经创建，直接返回。
	if !tx.Migrator().HasTable(&model.Skill{}) {
		return nil
	}
	oldSkills, err := loadSkillSystemMigrationSkills(tx)
	if err != nil {
		return err
	}
	if len(oldSkills) == 0 {
		return nil
	}
	skillByID := make(map[string]model.Skill, len(oldSkills))
	for _, skill := range oldSkills {
		skillByID[skill.ID] = skill
	}

	if err := migrateSkillSystemContents(tx, oldSkills); err != nil {
		return err
	}
	categoryMap, err := migrateSkillSystemCategories(tx)
	if err != nil {
		return err
	}
	if err := migrateSkillSystemListings(tx, oldSkills); err != nil {
		return err
	}

	var states []model.LegacyUserSkillState
	if tx.Migrator().HasTable(&model.LegacyUserSkillState{}) {
		if err := tx.Find(&states).Error; err != nil {
			return err
		}
	}
	if err := migrateSkillSystemInstalls(tx, oldSkills, skillByID, states, categoryMap); err != nil {
		return err
	}
	return migrateSkillSystemFavorites(tx, skillByID, states)
}

func loadSkillSystemMigrationSkills(tx *gorm.DB) ([]model.Skill, error) {
	if !tx.Migrator().HasColumn("skills", "current_version_id") {
		var rows []model.Skill
		return rows, tx.Find(&rows).Error
	}
	var legacy []legacySkillRow
	if err := tx.Find(&legacy).Error; err != nil {
		return nil, err
	}
	rows := make([]model.Skill, len(legacy))
	for index := range legacy {
		rows[index] = legacy[index].skill()
	}
	return rows, nil
}

func skillSystemContentID(skill model.Skill) string {
	if value := skill.ContentID; value != "" {
		return value
	}
	// 早期只有 instruction 的技能没有内容 ID；用技能 ID 作为稳定的内容 ID，
	// 后续导入任务可以从 instruction 重建内容。
	return skill.ID
}

func migrateSkillSystemContents(tx *gorm.DB, skills []model.Skill) error {
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		contentID := skillSystemContentID(skill)
		if _, exists := seen[contentID]; exists {
			continue
		}
		seen[contentID] = struct{}{}
		contentHash := skill.ContentHash
		size := skill.TotalBytes
		if contentHash == "" && skill.Instruction != "" {
			sum := sha256.Sum256([]byte(skill.Instruction))
			contentHash = hex.EncodeToString(sum[:])
			size = int64(len(skill.Instruction))
		}
		createdAt := skill.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		row := model.SkillContent{ID: contentID, Format: "zip", Size: size, SHA256: contentHash, CreatedAt: createdAt}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateSkillSystemCategories(tx *gorm.DB) (map[string]string, error) {
	if !tx.Migrator().HasTable(&model.LegacySkillLibraryCategory{}) {
		return map[string]string{}, nil
	}
	var categories []model.LegacySkillLibraryCategory
	if err := tx.Find(&categories).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(categories))
	for _, category := range categories {
		// 平台分类归后台 curation，不迁移到个人分类表。
		if category.Scope != "personal" || category.OwnerID == "" {
			continue
		}
		var existing model.UserSkillLibraryCategory
		err := tx.Where("user_id = ? AND normalized_name = ?", category.OwnerID, category.NormalizedName).First(&existing).Error
		if err == nil {
			result[category.ID] = existing.ID
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		row := model.UserSkillLibraryCategory{
			ID: kernel.NewID(), UserID: category.OwnerID, Name: category.Name, NormalizedName: category.NormalizedName,
			CreatedAt: category.CreatedAt, UpdatedAt: category.UpdatedAt,
		}
		if row.CreatedAt.IsZero() {
			row.CreatedAt = time.Now()
		}
		if row.UpdatedAt.IsZero() {
			row.UpdatedAt = row.CreatedAt
		}
		if err := tx.Create(&row).Error; err != nil {
			return nil, err
		}
		result[category.ID] = row.ID
	}
	return result, nil
}

func migrateSkillSystemListings(tx *gorm.DB, skills []model.Skill) error {
	for _, skill := range skills {
		if skill.IsPrivate {
			continue
		}
		status := "published"
		if skill.Status != 1 {
			status = "hidden"
		}
		showcase := skill.ShowcaseMediaJSON
		if showcase == "" {
			showcase = "[]"
		}
		listing := model.MarketSkillListing{
			SkillID: skill.ID, Status: status, SortWeight: skill.SortWeight,
			ShowcaseMediaJSON: showcase, ExtraInfo: skill.ExtraInfo,
			CreatedAt: skill.CreatedAt, UpdatedAt: skill.UpdatedAt,
		}
		if status == "published" {
			publishedAt := skill.UpdatedAt
			if publishedAt.IsZero() {
				publishedAt = time.Now()
			}
			listing.PublishedAt = &publishedAt
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "skill_id"}}, DoNothing: true}).Create(&listing).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateSkillSystemInstalls(tx *gorm.DB, skills []model.Skill, skillByID map[string]model.Skill, states []model.LegacyUserSkillState, categoryMap map[string]string) error {
	seen := make(map[string]struct{}, len(states)+len(skills))
	stateByUserSkill := make(map[string]model.LegacyUserSkillState, len(states))
	for _, state := range states {
		stateByUserSkill[state.UserID+"\x00"+state.SkillID] = state
	}
	create := func(userID string, skill model.Skill, state *model.LegacyUserSkillState) error {
		key := userID + "\x00" + skill.ID
		if _, exists := seen[key]; exists {
			return nil
		}
		seen[key] = struct{}{}
		owned := skill.OwnerID == userID
		var sourceSkillID *string
		if !owned {
			value := skill.ID
			sourceSkillID = &value
		}
		follow := !owned
		name, description, versionLabel := skill.Name, skill.Description, skill.VersionLabel
		agentMode := model.UserSkillAgentModeAuto
		var useCount int64
		var lastUsedAt *time.Time
		installedAt := skill.CreatedAt
		categoryID := ""
		if state != nil {
			if state.LibraryCategoryID != "" {
				categoryID = categoryMap[state.LibraryCategoryID]
			}
			if state.AgentMode == model.UserSkillAgentModeManual {
				agentMode = model.UserSkillAgentModeManual
			}
			useCount = state.AgentUseCount
			lastUsedAt = state.AgentLastUsedAt
			if !state.CreatedAt.IsZero() {
				installedAt = state.CreatedAt
			}
		}
		if installedAt.IsZero() {
			installedAt = time.Now()
		}
		instanceID := kernel.NewID()
		if owned {
			// 自有技能用原技能 ID 作为实例 ID，重复执行迁移时由主键冲突跳过。
			instanceID = skill.ID
		}
		row := model.UserInstalledSkill{
			ID: instanceID, UserID: userID, SourceSkillID: sourceSkillID,
			Name: name, Description: description, VersionLabel: versionLabel, VersionNumber: 1,
			ContentID: skillSystemContentID(skill), ContentHash: skill.ContentHash,
			FollowUpdates: follow, FollowMetadata: follow,
			LibraryCategoryID: categoryID, AgentMode: agentMode, AgentUseCount: useCount, AgentLastUsedAt: lastUsedAt,
			InstalledAt: installedAt, CreatedAt: installedAt, UpdatedAt: installedAt,
		}
		if row.UpdatedAt.IsZero() {
			row.UpdatedAt = row.CreatedAt
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	}

	for _, state := range states {
		skill, ok := skillByID[state.SkillID]
		if !ok {
			continue
		}
		if !state.Added && skill.OwnerID != state.UserID {
			continue
		}
		value := state
		if err := create(state.UserID, skill, &value); err != nil {
			return err
		}
	}
	for _, skill := range skills {
		// 只有用户私有作品自动成为安装实例；公开目录技能属于平台/管理员，不自动安装。
		if skill.OwnerID == "" || !skill.IsPrivate {
			continue
		}
		if _, exists := stateByUserSkill[skill.OwnerID+"\x00"+skill.ID]; exists {
			continue
		}
		if err := create(skill.OwnerID, skill, nil); err != nil {
			return err
		}
	}
	return nil
}

func migrateSkillSystemFavorites(tx *gorm.DB, skillByID map[string]model.Skill, states []model.LegacyUserSkillState) error {
	for _, state := range states {
		if !state.Liked {
			continue
		}
		skill, ok := skillByID[state.SkillID]
		if !ok || skill.IsPrivate {
			continue
		}
		row := model.UserSkillFavorite{UserID: state.UserID, SkillID: state.SkillID, CreatedAt: state.CreatedAt}
		if row.CreatedAt.IsZero() {
			row.CreatedAt = time.Now()
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// dropSQLiteIndexesOnColumn 删除引用指定列的索引。SQLite 的 DROP COLUMN 会被这类索引挡住；
// Postgres 删列时会连带删除索引，所以只在 sqlite 方言下处理。
func dropSQLiteIndexesOnColumn(tx *gorm.DB, table, column string) error {
	if tx.Dialector.Name() != "sqlite" {
		return nil
	}
	var names []string
	if err := tx.Raw("SELECT name FROM pragma_index_list(?) WHERE origin <> 'pk'", table).Scan(&names).Error; err != nil {
		return err
	}
	for _, name := range names {
		var columns []string
		if err := tx.Raw("SELECT name FROM pragma_index_info(?)", name).Scan(&columns).Error; err != nil {
			return err
		}
		for _, value := range columns {
			if value != column {
				continue
			}
			quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
			if err := tx.Exec("DROP INDEX IF EXISTS " + quoted).Error; err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// migrateSkillContentPointer 把公共目录的内容指针从 skill_versions 切到 skill_contents，
// 然后删除已被内容快照取代的版本表。内容 BLOB 仍由启动导入任务从旧 ZIP 或 instruction 补齐。
func migrateSkillContentPointer(tx *gorm.DB) error {
	if tx.Migrator().HasTable(&model.Skill{}) {
		if !tx.Migrator().HasColumn(&model.Skill{}, "ContentID") {
			if err := tx.Migrator().AddColumn(&model.Skill{}, "ContentID"); err != nil {
				return err
			}
		}
		if !tx.Migrator().HasIndex(&model.Skill{}, "ContentID") {
			if err := tx.Migrator().CreateIndex(&model.Skill{}, "ContentID"); err != nil {
				return err
			}
		}
		if tx.Migrator().HasColumn("skills", "current_version_id") {
			if err := tx.Exec("UPDATE skills SET content_id = current_version_id WHERE (content_id IS NULL OR content_id = '') AND current_version_id <> ''").Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE skills SET content_id = id WHERE content_id IS NULL OR content_id = ''").Error; err != nil {
			return err
		}
		if tx.Migrator().HasColumn("skills", "current_version_id") {
			// SQLite 在索引仍引用该列时会拒绝 DROP COLUMN；旧模型曾给这列建过索引，
			// 不先删索引会让升级在迁移 110 直接失败（Postgres 会随列自动删索引）。
			if err := dropSQLiteIndexesOnColumn(tx, "skills", "current_version_id"); err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE skills DROP COLUMN current_version_id").Error; err != nil {
				return err
			}
		}
	}
	for _, table := range []string{"skill_files", "skill_versions"} {
		if tx.Migrator().HasTable(table) {
			if err := tx.Migrator().DropTable(table); err != nil {
				return err
			}
		}
	}
	return nil
}
