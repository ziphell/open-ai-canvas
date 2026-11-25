package database

import (
	"time"

	"yingce/backend/internal/model"
)

// legacySkillRow 映射迁移 110 之前的 skills 表结构。
// 迁移 106 在旧数据库上运行时，模型已改用 content_id，必须用这份结构读取 current_version_id。
type legacySkillRow struct {
	ID                   string
	OwnerID              string
	AuthorName           string
	AuthorAvatarURL      string
	Name                 string
	Description          string
	Instruction          string
	CurrentVersionID     string
	VersionLabel         string
	ContentHash          string
	FileCount            int
	TotalBytes           int64
	SourceType           string
	SourceURL            string
	SourceRef            string
	SourceSubdir         string
	SourceCommit         string
	SyncStatus           string
	SyncError            string
	AutoUpdate           bool
	LastCheckedAt        *time.Time
	LastSyncedAt         *time.Time
	Status               int
	Source               int
	Tag                  string
	SortWeight           int
	IsPrivate            bool
	MarkdownURL          string
	ShowcaseMediaJSON    string
	ExtraInfo            string
	InitialLikeCount     int64
	InitialAddedCount    int64
	AdminInfoEditedAt    *time.Time
	AdminContentEditedAt *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (legacySkillRow) TableName() string { return "skills" }

func (row legacySkillRow) skill() model.Skill {
	return model.Skill{
		ID: row.ID, OwnerID: row.OwnerID, AuthorName: row.AuthorName, AuthorAvatarURL: row.AuthorAvatarURL,
		Name: row.Name, Description: row.Description, Instruction: row.Instruction,
		ContentID: row.CurrentVersionID, VersionLabel: row.VersionLabel, ContentHash: row.ContentHash,
		FileCount: row.FileCount, TotalBytes: row.TotalBytes,
		SourceType: row.SourceType, SourceURL: row.SourceURL, SourceRef: row.SourceRef, SourceSubdir: row.SourceSubdir, SourceCommit: row.SourceCommit,
		SyncStatus: row.SyncStatus, SyncError: row.SyncError, AutoUpdate: row.AutoUpdate,
		LastCheckedAt: row.LastCheckedAt, LastSyncedAt: row.LastSyncedAt,
		Status: row.Status, Source: row.Source, Tag: row.Tag, SortWeight: row.SortWeight,
		IsPrivate: row.IsPrivate, MarkdownURL: row.MarkdownURL, ShowcaseMediaJSON: row.ShowcaseMediaJSON, ExtraInfo: row.ExtraInfo,
		InitialLikeCount: row.InitialLikeCount, InitialAddedCount: row.InitialAddedCount,
		AdminInfoEditedAt: row.AdminInfoEditedAt, AdminContentEditedAt: row.AdminContentEditedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
