package model

import "time"

// 以下类型只用于重放历史迁移 39 / 40。对应表已由迁移 109 删除，
// 运行时任何代码都不应再读写它们；新的用户技能关系见 skill_system.go。

type LegacySkillLibraryCategory struct {
	ID             string `gorm:"primaryKey;size:36"`
	Name           string `gorm:"size:64;not null"`
	NormalizedName string `gorm:"size:96;not null;uniqueIndex:idx_skill_library_category_scope_owner_name,priority:3"`
	Scope          string `gorm:"size:16;not null;uniqueIndex:idx_skill_library_category_scope_owner_name,priority:1;index"`
	OwnerID        string `gorm:"size:36;not null;uniqueIndex:idx_skill_library_category_scope_owner_name,priority:2;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (LegacySkillLibraryCategory) TableName() string { return "skill_library_categories" }

type LegacyUserSkillState struct {
	ID                 string `gorm:"primaryKey;size:36"`
	UserID             string `gorm:"size:36;index;uniqueIndex:idx_user_skill_state_user_skill,priority:1"`
	SkillID            string `gorm:"size:36;index;uniqueIndex:idx_user_skill_state_user_skill,priority:2"`
	LibraryCategoryID  string `gorm:"size:36;index"`
	InstalledVersionID string `gorm:"size:36;index"`
	AutoUpdate         bool
	Added              bool   `gorm:"index"`
	Liked              bool   `gorm:"index"`
	AgentMode          string `gorm:"size:16"`
	AgentUseCount      int64
	AgentLastUsedAt    *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (LegacyUserSkillState) TableName() string { return "user_skill_states" }

type LegacyBuiltinSkillTombstone struct {
	SkillID   string `gorm:"primaryKey;size:36"`
	DeletedBy string `gorm:"size:36"`
	DeletedAt time.Time
}

func (LegacyBuiltinSkillTombstone) TableName() string { return "builtin_skill_tombstones" }

// LegacySkillRelationshipTables 重放迁移 39。
func LegacySkillRelationshipTables() []any {
	return []any{&LegacySkillLibraryCategory{}, &LegacyUserSkillState{}}
}

// LegacyBuiltinSkillTombstoneTable 重放迁移 40。
func LegacyBuiltinSkillTombstoneTable() any { return &LegacyBuiltinSkillTombstone{} }

// LegacySkillTables 供迁移 109 删除历史表。
func LegacySkillTables() []any {
	return []any{
		&LegacyUserSkillState{},
		&LegacySkillLibraryCategory{},
		&LegacyBuiltinSkillTombstone{},
	}
}
