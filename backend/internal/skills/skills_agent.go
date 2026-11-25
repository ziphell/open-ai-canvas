package skills

import (
	"path"
	"strconv"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

// AgentSkillCatalogEntry 是 Agent 技能目录与检索使用的技能元数据，不含正文。
type AgentSkillCatalogEntry struct {
	SkillID     string     `json:"skillId"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tag         string     `json:"tag"`
	VersionID   string     `json:"versionId"`
	Version     string     `json:"version"`
	ContentHash string     `json:"contentHash"`
	FileCount   int        `json:"fileCount"`
	SortWeight  int        `json:"sortWeight"`
	Owned       bool       `json:"owned"`
	Installed   bool       `json:"installed"`
	AgentMode   string     `json:"agentMode"`
	UseCount    int64      `json:"useCount"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	InstalledAt *time.Time `json:"installedAt,omitempty"`
	AddedCount  int64      `json:"addedCount,omitempty"`
}

// NormalizeSkillAgentMode 把空值和未知值都归为 auto，只有显式 manual 才退出自动发现。
func NormalizeSkillAgentMode(mode string) string {
	if strings.TrimSpace(mode) == model.UserSkillAgentModeManual {
		return model.UserSkillAgentModeManual
	}
	return model.UserSkillAgentModeAuto
}

func (s *Service) agentSkillEntry(userID string, instance *model.UserInstalledSkill) AgentSkillCatalogEntry {
	var source *model.Skill
	if instance.SourceSkillID != nil {
		source, _ = s.repo.SkillAnyStatus(*instance.SourceSkillID)
	}
	name, description := s.displayNameAndDescription(instance, source)
	tag, sortWeight := "", 0
	if source != nil {
		tag, sortWeight = source.Tag, source.SortWeight
	}
	version := instance.VersionLabel
	if version == "" {
		version = strconv.Itoa(instance.VersionNumber)
	}
	installedAt := instance.InstalledAt
	return AgentSkillCatalogEntry{
		SkillID: instance.ID, Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), Tag: tag,
		VersionID: instance.ContentID, Version: version, ContentHash: instance.ContentHash, FileCount: instance.FileCount,
		SortWeight: sortWeight, Owned: instance.SourceSkillID == nil, Installed: true,
		AgentMode: NormalizeSkillAgentMode(instance.AgentMode), UseCount: instance.AgentUseCount,
		LastUsedAt: instance.AgentLastUsedAt, InstalledAt: &installedAt,
	}
}

// AgentSkillCatalog 返回用户自己的技能实例（上传/创建 + 从广场安装）。
func (s *Service) AgentSkillCatalog(userID string) ([]AgentSkillCatalogEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	rows, err := s.repo.UserInstalledSkillsAll(userID)
	if err != nil {
		return nil, err
	}
	entries := make([]AgentSkillCatalogEntry, 0, len(rows))
	for index := range rows {
		if strings.TrimSpace(rows[index].ContentID) == "" {
			continue
		}
		entries = append(entries, s.agentSkillEntry(userID, &rows[index]))
	}
	return entries, nil
}

// AgentSkillCatalogByIDs 只返回当前用户仍拥有的指定技能实例。
func (s *Service) AgentSkillCatalogByIDs(userID string, ids []string) ([]AgentSkillCatalogEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if len(ids) == 0 {
		return []AgentSkillCatalogEntry{}, nil
	}
	rows, err := s.repo.UserInstalledSkillsByIDs(userID, ids)
	if err != nil {
		return nil, err
	}
	entries := make([]AgentSkillCatalogEntry, 0, len(rows))
	for index := range rows {
		entries = append(entries, s.agentSkillEntry(userID, &rows[index]))
	}
	return entries, nil
}

// MarketSkillCatalog 返回用户还没安装的已发布广场技能，只用于推荐，不含正文。
func (s *Service) MarketSkillCatalog(userID string) ([]AgentSkillCatalogEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	rows, _, err := s.repo.MarketSkills(repository.MarketSkillFilter{UserID: userID, ExcludeOwnerID: userID, Sort: "popular", Limit: 2000})
	if err != nil {
		return nil, err
	}
	entries := make([]AgentSkillCatalogEntry, 0, len(rows))
	for _, row := range rows {
		if row.InstanceID != "" {
			continue
		}
		version := row.VersionLabel
		if version == "" {
			version = "1"
		}
		entries = append(entries, AgentSkillCatalogEntry{
			SkillID: row.SkillID, Name: strings.TrimSpace(row.Name), Description: strings.TrimSpace(row.Description),
			Tag: row.Tag, VersionID: row.ContentID, Version: version, ContentHash: row.ContentHash,
			FileCount: row.FileCount, SortWeight: row.SortWeight, Installed: false,
			AgentMode: model.UserSkillAgentModeAuto, AddedCount: row.AddedCount,
		})
	}
	return entries, nil
}

// SetSkillAgentMode 设置用户技能实例是否允许 Agent 自动发现。
func (s *Service) SetSkillAgentMode(userID, id, mode string) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	mode = strings.TrimSpace(mode)
	if mode != model.UserSkillAgentModeAuto && mode != model.UserSkillAgentModeManual {
		return nil, kernel.BadAuthRequest("Agent 使用方式只能是 auto 或 manual")
	}
	instance, err := s.repo.UserInstalledSkill(userID, id)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		instance, err = s.repo.UserInstalledSkillBySource(userID, id)
		if err != nil {
			return nil, err
		}
	}
	if instance == nil {
		return nil, kernel.NotFound("请先安装该技能，再设置 Agent 使用方式")
	}
	if err := s.repo.UpdateUserInstalledSkill(instance.ID, map[string]any{"agent_mode": mode, "updated_at": time.Now()}); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, updated, false)
}

// RecordSkillAgentUse 记录 Agent 本轮实际读取过的技能实例，用于目录排序。
func (s *Service) RecordSkillAgentUse(userID string, skillIDs []string) error {
	if strings.TrimSpace(userID) == "" {
		return kernel.Unauthorized("请先登录")
	}
	return s.repo.RecordUserInstalledSkillAgentUse(userID, skillIDs, time.Now())
}

// SkillContentTextPaths 返回内容快照中可供 Agent 读取的文本文件路径（含 SKILL.md）。
func (s *Service) SkillContentTextPaths(contentIDs []string) (map[string][]string, error) {
	rows, err := s.repo.SkillContentFilePaths(contentIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(rows))
	for contentID, paths := range rows {
		readable := make([]string, 0, len(paths))
		for _, filePath := range paths {
			if AgentReadableSkillPath(filePath) {
				readable = append(readable, filePath)
			}
		}
		result[contentID] = readable
	}
	return result, nil
}

// AgentReadableSkillPath 是 Agent 可读文件的唯一判定：只读 Markdown、纯文本和 JSON。
func AgentReadableSkillPath(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".txt", ".json":
		return true
	default:
		return false
	}
}
