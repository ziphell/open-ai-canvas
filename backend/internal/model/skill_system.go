package model

import "time"

// SkillContent 是不可变的技能包内容快照。多个技能或用户实例可以共享同一条内容。
// 更新内容时新增一行并切换引用，不原地修改。
type SkillContent struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	Format    string    `json:"format" gorm:"size:16;not null;default:zip"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256" gorm:"size:64;index"`
	Data      []byte    `json:"-" gorm:"column:data"`
	CreatedAt time.Time `json:"createdAt"`
}

func (SkillContent) TableName() string { return "skill_contents" }

// SkillContentFile 是内容快照内的文件索引。Phase 2 暂时使用独立表名，
// 等旧 skill_files 清理后再统一改名为 skill_files。
type SkillContentFile struct {
	ID        string    `json:"id" gorm:"primaryKey;size:36"`
	ContentID string    `json:"contentId" gorm:"size:36;not null;index;uniqueIndex:idx_skill_content_file_path,priority:1"`
	Path      string    `json:"path" gorm:"size:1000;not null;uniqueIndex:idx_skill_content_file_path,priority:2"`
	Kind      string    `json:"kind" gorm:"size:24;index"`
	MimeType  string    `json:"mimeType" gorm:"size:255"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256" gorm:"size:64"`
	CreatedAt time.Time `json:"createdAt"`
}

func (SkillContentFile) TableName() string { return "skill_content_files" }

// MarketSkillListing 是技能广场的发布记录。默认没有记录，管理员发布后才有。
type MarketSkillListing struct {
	SkillID           string     `json:"skillId" gorm:"primaryKey;size:36"`
	Status            string     `json:"status" gorm:"size:16;not null;default:draft;index:idx_market_skill_listing_status_sort,priority:1"`
	SortWeight        int        `json:"sortWeight" gorm:"not null;default:0;index:idx_market_skill_listing_status_sort,priority:2"`
	ShowcaseMediaJSON string     `json:"-" gorm:"type:text;not null;default:'[]'"`
	ExtraInfo         string     `json:"extraInfo" gorm:"type:text;not null;default:''"`
	PublishedAt       *time.Time `json:"publishedAt,omitempty"`
	PublishedBy       string     `json:"publishedBy,omitempty" gorm:"size:36"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

func (MarketSkillListing) TableName() string { return "market_skill_listings" }

// UserInstalledSkill 是用户自己的技能实例：
//   - 从广场安装：source_skill_id 指向广场技能，content_id 指向安装时快照
//   - 用户上传/创建：source_skill_id 为空，content_id 指向自己的内容
//
// 名称和简介保存快照，follow_metadata 决定未覆盖时是否显示广场当前值。
type UserInstalledSkill struct {
	ID            string  `json:"id" gorm:"primaryKey;size:36"`
	UserID        string  `json:"userId" gorm:"size:36;not null;index;uniqueIndex:idx_user_installed_skill_user_source,priority:1"`
	SourceSkillID *string `json:"sourceSkillId,omitempty" gorm:"size:36;index;uniqueIndex:idx_user_installed_skill_user_source,priority:2"`

	Name          string `json:"name" gorm:"size:80;not null"`
	Description   string `json:"description" gorm:"size:500;not null"`
	VersionLabel  string `json:"versionLabel,omitempty" gorm:"size:64"`
	VersionNumber int    `json:"versionNumber" gorm:"not null;default:1"`

	ContentID   string `json:"contentId" gorm:"size:36;not null;index"`
	ContentHash string `json:"contentHash" gorm:"size:64;not null;index"`
	FileCount   int    `json:"fileCount"`
	TotalBytes  int64  `json:"totalBytes"`

	SourceType    string     `json:"sourceType,omitempty" gorm:"size:24;index"`
	SourceURL     string     `json:"sourceUrl,omitempty" gorm:"size:1000"`
	SourceRef     string     `json:"sourceRef,omitempty" gorm:"size:255"`
	SourceSubdir  string     `json:"sourceSubdir,omitempty" gorm:"size:1000"`
	SourceCommit  string     `json:"sourceCommit,omitempty" gorm:"size:64"`
	SyncStatus    string     `json:"syncStatus,omitempty" gorm:"size:24"`
	SyncError     string     `json:"syncError,omitempty" gorm:"type:text"`
	AutoUpdate    bool       `json:"autoUpdate" gorm:"index"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt,omitempty"`

	NameOverride        string `json:"nameOverride,omitempty" gorm:"size:80"`
	DescriptionOverride string `json:"descriptionOverride,omitempty" gorm:"size:500"`
	FollowUpdates       bool   `json:"followUpdates" gorm:"not null;default:false"`
	FollowMetadata      bool   `json:"followMetadata" gorm:"not null;default:false"`

	LibraryCategoryID string `json:"libraryCategoryId,omitempty" gorm:"size:36;index"`

	AgentMode       string     `json:"agentMode" gorm:"size:16;not null;default:auto"`
	AgentUseCount   int64      `json:"agentUseCount" gorm:"not null;default:0"`
	AgentLastUsedAt *time.Time `json:"agentLastUsedAt,omitempty"`

	InstalledAt time.Time `json:"installedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (UserInstalledSkill) TableName() string { return "user_installed_skills" }

// UserSkillFavorite 收藏指向广场发布记录，私有技能没有 listing，因此不可收藏。
type UserSkillFavorite struct {
	UserID    string    `json:"userId" gorm:"primaryKey;size:36"`
	SkillID   string    `json:"skillId" gorm:"primaryKey;size:36;index"`
	CreatedAt time.Time `json:"createdAt"`
}

func (UserSkillFavorite) TableName() string { return "user_skill_favorites" }

// UserSkillLibraryCategory 只保存个人技能库分类，平台分类归后台 curation。
type UserSkillLibraryCategory struct {
	ID             string    `json:"id" gorm:"primaryKey;size:36"`
	UserID         string    `json:"-" gorm:"size:36;not null;index;uniqueIndex:idx_user_skill_library_category_name,priority:1"`
	Name           string    `json:"name" gorm:"size:64;not null"`
	NormalizedName string    `json:"-" gorm:"size:96;not null;uniqueIndex:idx_user_skill_library_category_name,priority:2"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (UserSkillLibraryCategory) TableName() string { return "user_skill_library_categories" }
