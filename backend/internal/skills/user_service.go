package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/gorm"
)

func (s *Service) ensureContentImported(contentID string) error {
	if strings.TrimSpace(contentID) == "" {
		return kernel.BadAuthRequest("技能内容 ID 不能为空")
	}
	exists, err := s.repo.SkillContentExists(contentID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	archive, err := s.loadArchiveForContent(contentID)
	if err != nil {
		return kernel.BadAuthRequest("技能内容尚未就绪，请稍后重试")
	}
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		return err
	}
	return s.repo.SaveSkillContentArchive(contentID, data, int64(len(data)), archive.ContentHash, s.contentFileRecords(contentID, archive.Files))
}

func (s *Service) contentFileRecords(contentID string, files map[string][]byte) []model.SkillContentFile {
	rows := make([]model.SkillContentFile, 0, len(files))
	now := time.Now()
	for path, content := range files {
		rows = append(rows, model.SkillContentFile{
			ID: kernel.NewID(), ContentID: contentID, Path: path, Kind: skillFileKind(path),
			MimeType: skillFileMime(path, content), Size: int64(len(content)),
			SHA256: skillContentSHA256(content), CreatedAt: now,
		})
	}
	return rows
}

func skillContentSHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// skillContentTotalBytes 返回内容快照解压后的文件总大小，用于展示和实例快照。
func skillContentTotalBytes(files []model.SkillContentFile) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

func (s *Service) installMarketSkill(userID, catalogID string) (*model.UserInstalledSkill, error) {
	catalogID = strings.TrimSpace(catalogID)
	if catalogID == "" {
		return nil, kernel.BadAuthRequest("技能 ID 不能为空")
	}
	skill, err := s.repo.SkillAnyStatus(catalogID)
	if err != nil {
		return nil, kernel.NotFound("技能不存在或已删除")
	}
	if skill.IsPrivate {
		return nil, kernel.Forbidden("该技能未公开")
	}
	visible, err := s.repo.MarketSkill(userID, catalogID)
	if err != nil {
		return nil, err
	}
	if visible == nil {
		return nil, kernel.Forbidden("该技能未上架，暂时不能安装")
	}
	if existing, err := s.repo.UserInstalledSkillBySource(userID, catalogID); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	contentID := strings.TrimSpace(skill.ContentID)
	if contentID == "" {
		contentID = skill.ID
	}
	if err := s.ensureContentImported(contentID); err != nil {
		return nil, err
	}
	content, err := s.repo.SkillContentWithData(contentID)
	if err != nil {
		return nil, err
	}
	files, err := s.repo.SkillContentFiles(contentID)
	if err != nil {
		return nil, err
	}
	contentHash := skill.ContentHash
	if content.SHA256 != "" {
		contentHash = content.SHA256
	}
	source := catalogID
	now := time.Now()
	instance := &model.UserInstalledSkill{
		ID: kernel.NewID(), UserID: userID, SourceSkillID: &source,
		Name: skill.Name, Description: skill.Description, VersionLabel: skill.VersionLabel, VersionNumber: 1,
		ContentID: contentID, ContentHash: contentHash, FileCount: len(files), TotalBytes: skillContentTotalBytes(files),
		FollowUpdates: true, FollowMetadata: true,
		SourceType: skill.SourceType, SourceURL: skill.SourceURL, SourceRef: skill.SourceRef,
		SourceSubdir: skill.SourceSubdir, SourceCommit: skill.SourceCommit,
		AgentMode: model.UserSkillAgentModeAuto, InstalledAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if instance.VersionLabel == "" {
		instance.VersionLabel = "1"
	}
	if err := s.repo.CreateUserInstalledSkill(instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) createPrivateUserSkill(userID string, archive skillPackageArchive, sourceType, sourceURL, sourceRef, sourceSubdir, sourceCommit string, autoUpdate bool, overrideName, overrideDescription string) (*model.UserInstalledSkill, error) {
	name := strings.TrimSpace(overrideName)
	if name == "" {
		name = strings.TrimSpace(archive.Metadata.Name)
	}
	description := strings.TrimSpace(overrideDescription)
	if description == "" {
		description = strings.TrimSpace(archive.Metadata.Description)
	}
	if utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 80 {
		return nil, kernel.BadAuthRequest("技能名称必须为 1-80 个字符")
	}
	if utf8.RuneCountInString(description) == 0 || utf8.RuneCountInString(description) > 500 {
		return nil, kernel.BadAuthRequest("技能简介必须为 1-500 个字符")
	}
	contentID := kernel.NewID()
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		return nil, err
	}
	files := s.contentFileRecords(contentID, archive.Files)
	versionLabel := strings.TrimSpace(archive.Metadata.Version)
	if versionLabel == "" {
		versionLabel = "1"
	}
	now := time.Now()
	instance := &model.UserInstalledSkill{
		ID: kernel.NewID(), UserID: userID, Name: name, Description: description,
		VersionLabel: versionLabel, VersionNumber: 1,
		ContentID: contentID, ContentHash: archive.ContentHash, FileCount: len(archive.Files), TotalBytes: archive.TotalBytes,
		FollowUpdates: false, FollowMetadata: false,
		SourceType: sourceType, SourceURL: sourceURL, SourceRef: sourceRef, SourceSubdir: sourceSubdir,
		SourceCommit: sourceCommit, AutoUpdate: autoUpdate, SyncStatus: "synced",
		AgentMode: model.UserSkillAgentModeAuto, InstalledAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if autoUpdate {
		instance.LastCheckedAt = &now
		instance.LastSyncedAt = &now
	}
	content := &model.SkillContent{ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data, CreatedAt: now}
	if err := s.repo.CreateUserSkillWithContent(instance, content, files); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) instanceSnapshot(instance *model.UserInstalledSkill) (*skillPackageSnapshot, error) {
	if instance == nil || strings.TrimSpace(instance.ContentID) == "" {
		return nil, kernel.BadAuthRequest("技能内容不存在")
	}
	fake := &model.Skill{ID: instance.ID, ContentID: instance.ContentID, ContentHash: instance.ContentHash}
	snapshot, ok := s.contentSnapshot(fake)
	if !ok {
		return nil, kernel.BadAuthRequest("技能内容尚未就绪")
	}
	return snapshot, nil
}

func (s *Service) displayNameAndDescription(instance *model.UserInstalledSkill, source *model.Skill) (string, string) {
	name, description := instance.Name, instance.Description
	if instance.NameOverride != "" {
		name = instance.NameOverride
	} else if instance.SourceSkillID != nil && instance.FollowMetadata && source != nil {
		name = source.Name
	}
	if instance.DescriptionOverride != "" {
		description = instance.DescriptionOverride
	} else if instance.SourceSkillID != nil && instance.FollowMetadata && source != nil {
		description = source.Description
	}
	return name, description
}

func (s *Service) effectiveUser(userID string) SkillEffectiveUser {
	user, err := s.repo.User(userID)
	if err != nil || user == nil {
		return SkillEffectiveUser{Name: "未知用户", UID: userID}
	}
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = strings.TrimSpace(user.Username)
	}
	avatars, _ := s.repo.SkillOwnerAvatars([]string{userID})
	return SkillEffectiveUser{Name: name, AvatarURL: avatars[userID], UID: userID}
}

func (s *Service) userSkillItem(userID string, instance *model.UserInstalledSkill, includeInstruction bool) (*SkillItem, error) {
	if instance == nil {
		return nil, kernel.NotFound("技能不存在")
	}
	var source *model.Skill
	if instance.SourceSkillID != nil {
		source, _ = s.repo.SkillAnyStatus(*instance.SourceSkillID)
	}
	name, description := s.displayNameAndDescription(instance, source)
	var addedCount, likeCount int64
	var isLike bool
	if source != nil {
		addedCount, likeCount, isLike, _ = s.repo.SkillSocialState(userID, source.ID)
	}
	tag, showcaseJSON, extraInfo, sortWeight, markdownURL := "", "[]", "", 0, ""
	if source != nil {
		tag, showcaseJSON, extraInfo, sortWeight, markdownURL = source.Tag, source.ShowcaseMediaJSON, source.ExtraInfo, source.SortWeight, source.MarkdownURL
	}
	sourceType, sourceURL, sourceRef, sourceSubdir, sourceCommit := instance.SourceType, instance.SourceURL, instance.SourceRef, instance.SourceSubdir, instance.SourceCommit
	syncStatus, syncError := instance.SyncStatus, instance.SyncError
	autoUpdate, lastCheckedAt, lastSyncedAt := instance.AutoUpdate, instance.LastCheckedAt, instance.LastSyncedAt
	if sourceType == "" && source != nil {
		sourceType, sourceURL, sourceRef, sourceSubdir, sourceCommit = source.SourceType, source.SourceURL, source.SourceRef, source.SourceSubdir, source.SourceCommit
	}
	if syncStatus == "" && source != nil {
		syncStatus, syncError = source.SyncStatus, source.SyncError
	}
	showcaseMedia, _ := parseShowcaseMedia(showcaseJSON)
	version := instance.VersionLabel
	if version == "" {
		version = fmt.Sprintf("%d", instance.VersionNumber)
	}
	item := &SkillItem{
		SkillID: instance.ID, SkillName: name, Description: description,
		VersionID: instance.ContentID, Version: version, ContentHash: instance.ContentHash,
		FileCount: instance.FileCount, TotalBytes: instance.TotalBytes,
		SourceType: sourceType, SourceURL: sourceURL, SourceRef: sourceRef, SourceSubdir: sourceSubdir, SourceCommit: sourceCommit,
		SyncStatus: syncStatus, SyncError: syncError, AutoUpdate: autoUpdate, LastCheckedAt: lastCheckedAt, LastSyncedAt: lastSyncedAt,
		Status: 1, MarkdownURL: markdownURL, CreatedAt: instance.CreatedAt, UpdatedAt: instance.UpdatedAt,
		Source: 1, Tag: tag, SortWeight: sortWeight, IsPrivate: true,
		LikeCount: likeCount, IsLike: isLike, OwnerUID: instance.UserID,
		ShowcaseMedia: showcaseMedia, AddedCount: addedCount, ExtraInfo: extraInfo,
		IsAdded: true, IsOwner: instance.SourceSkillID == nil,
		FollowUpdates: instance.FollowUpdates, FollowMetadata: instance.FollowMetadata,
		LibraryCategoryID: instance.LibraryCategoryID,
		AgentMode:         NormalizeSkillAgentMode(instance.AgentMode),
	}
	item.HasUpdate = source != nil && instance.FollowUpdates && source.ContentHash != "" && source.ContentHash != instance.ContentHash
	ownerID := instance.UserID
	if source != nil && source.OwnerID != "" {
		ownerID = source.OwnerID
	}
	item.EffectiveUser = s.effectiveUser(ownerID)
	if source != nil {
		value := source.ID
		item.OriginalSkillID = &value
	}
	if includeInstruction {
		snapshot, err := s.instanceSnapshot(instance)
		if err != nil {
			return nil, err
		}
		entry, err := s.readSkillPackageSnapshotEntry(snapshot, "SKILL.md")
		if err != nil {
			return nil, err
		}
		item.Instruction = string(entry)
		item.FileCount = len(snapshot.files)
		item.TotalBytes = snapshot.version.TotalBytes
	}
	return item, nil
}

func (s *Service) marketSkillItem(userID string, row repository.MarketSkillRow, includeInstruction bool) (*SkillItem, error) {
	showcaseMedia, _ := parseShowcaseMedia(row.ShowcaseMediaJSON)
	version := row.VersionLabel
	if version == "" {
		version = "1"
	}
	status := 0
	if row.ListingStatus == "published" {
		status = 1
	}
	item := &SkillItem{
		SkillID: row.SkillID, SkillName: row.Name, Description: row.Description,
		VersionID: row.ContentID, Version: version, ContentHash: row.ContentHash,
		FileCount: row.FileCount, TotalBytes: row.TotalBytes,
		SourceType: row.SourceType, SourceURL: row.SourceURL, SourceRef: row.SourceRef,
		SourceSubdir: row.SourceSubdir, SourceCommit: row.SourceCommit,
		Status: status, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		Source: 1, Tag: row.Tag, SortWeight: row.SortWeight, IsPrivate: false,
		LikeCount: row.LikeCount, IsLike: row.IsLike, OwnerUID: row.OwnerID,
		ShowcaseMedia: showcaseMedia, AddedCount: row.AddedCount, ExtraInfo: row.ExtraInfo,
		IsAdded: row.InstanceID != "", IsOwner: false,
	}
	item.EffectiveUser = s.effectiveUser(row.OwnerID)
	if includeInstruction && row.ContentID != "" {
		if err := s.ensureContentImported(row.ContentID); err == nil {
			fake := &model.Skill{ID: row.SkillID, ContentID: row.ContentID, ContentHash: row.ContentHash}
			if snapshot, ok := s.contentSnapshot(fake); ok {
				if entry, err := s.readSkillPackageSnapshotEntry(snapshot, "SKILL.md"); err == nil {
					item.Instruction = string(entry)
					item.FileCount = len(snapshot.files)
					item.TotalBytes = snapshot.version.TotalBytes
				}
			}
		}
	}
	return item, nil
}

// UpdateUserSkillFromMarket 把未编辑的安装实例更新到广场当前内容。
func (s *Service) UpdateUserSkillFromMarket(userID, instanceID string) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	instance, err := s.repo.UserInstalledSkill(userID, instanceID)
	if err != nil {
		return nil, err
	}
	if instance == nil {
		return nil, kernel.NotFound("技能不存在")
	}
	if instance.SourceSkillID == nil {
		return nil, kernel.BadAuthRequest("自建技能不跟随广场更新")
	}
	if !instance.FollowUpdates {
		return nil, kernel.BadAuthRequest("技能已编辑，不再跟随广场更新")
	}
	sourceID := *instance.SourceSkillID
	row, err := s.repo.MarketSkill(userID, sourceID)
	if err != nil || row == nil {
		return nil, kernel.Forbidden("来源技能已下架，暂时不能更新")
	}
	skill, err := s.repo.SkillAnyStatus(sourceID)
	if err != nil {
		return nil, err
	}
	contentID := strings.TrimSpace(skill.ContentID)
	if contentID == "" {
		contentID = skill.ID
	}
	if err := s.ensureContentImported(contentID); err != nil {
		return nil, err
	}
	content, err := s.repo.SkillContentWithData(contentID)
	if err != nil {
		return nil, err
	}
	files, err := s.repo.SkillContentFiles(contentID)
	if err != nil {
		return nil, err
	}
	contentHash := skill.ContentHash
	if content.SHA256 != "" {
		contentHash = content.SHA256
	}
	fields := map[string]any{
		"content_id": contentID, "content_hash": contentHash,
		"version_label": skill.VersionLabel, "file_count": len(files), "total_bytes": skillContentTotalBytes(files),
		"updated_at": time.Now(),
	}
	if !instance.FollowMetadata {
		fields["name_override"], fields["description_override"] = instance.NameOverride, instance.DescriptionOverride
	} else {
		fields["name"], fields["description"] = skill.Name, skill.Description
	}
	if err := s.repo.UpdateUserInstalledSkill(instance.ID, fields); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil || updated == nil {
		return nil, err
	}
	return s.userSkillItem(userID, updated, true)
}

func (s *Service) syncUserGitHubSkill(userID string, instance *model.UserInstalledSkill) (*SkillItem, error) {
	if instance == nil || instance.SourceType != "github" || strings.TrimSpace(instance.SourceURL) == "" {
		return nil, kernel.BadAuthRequest("只有 GitHub 技能可以同步")
	}
	spec, err := parseGitHubSkillURL(instance.SourceURL, instance.SourceRef, instance.SourceSubdir)
	if err != nil {
		return nil, err
	}
	archive, commit, canonicalURL, resolvedRef, err := fetchGitHubSkillArchive(context.Background(), spec)
	now := time.Now()
	if err != nil {
		_ = s.repo.UpdateUserInstalledSkill(instance.ID, map[string]any{"sync_status": "failed", "sync_error": err.Error(), "last_checked_at": now, "updated_at": now})
		return nil, err
	}
	if commit != "" && commit == instance.SourceCommit {
		_ = s.repo.UpdateUserInstalledSkill(instance.ID, map[string]any{"sync_status": "synced", "sync_error": "", "last_checked_at": now, "updated_at": now})
		return s.userSkillItem(userID, instance, true)
	}
	contentID := kernel.NewID()
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		return nil, err
	}
	content := &model.SkillContent{ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data, CreatedAt: now}
	files := s.contentFileRecords(contentID, archive.Files)
	fields := map[string]any{
		"content_id": contentID, "content_hash": archive.ContentHash,
		"version_number": gorm.Expr("version_number + 1"), "version_label": archive.Metadata.Version,
		"file_count": len(archive.Files), "total_bytes": archive.TotalBytes,
		"source_url": canonicalURL, "source_ref": resolvedRef, "source_commit": commit,
		"sync_status": "synced", "sync_error": "", "last_checked_at": now, "last_synced_at": now,
		"updated_at": now,
	}
	if fields["version_label"] == "" {
		fields["version_label"] = editedSkillVersionLabel()
	}
	if err := s.repo.ReplaceUserSkillContent(instance.ID, instance.ContentID, fields, content, files); err != nil {
		return nil, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil || updated == nil {
		return nil, err
	}
	return s.userSkillItem(userID, updated, true)
}

// visibleCatalogSkill 允许普通用户读取已发布广场技能或自己的私有旧技能文件。
func (s *Service) visibleCatalogSkill(userID, skillID string) (*model.Skill, error) {
	skill, err := s.repo.SkillAnyStatus(strings.TrimSpace(skillID))
	if err != nil {
		return nil, kernel.NotFound("技能不存在或已删除")
	}
	if skill.IsPrivate {
		if skill.OwnerID != userID {
			return nil, kernel.Forbidden("该技能未公开")
		}
		return skill, nil
	}
	row, err := s.repo.MarketSkill(userID, skill.ID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, kernel.Forbidden("该技能未上架")
	}
	return skill, nil
}

func (s *Service) editUserSkillPackage(userID, instanceID string, req SkillPackageEditRequest) (*SkillPackageEditResult, bool, error) {
	instance, err := s.repo.UserInstalledSkill(userID, instanceID)
	if err != nil {
		return nil, false, err
	}
	if instance == nil {
		if other, err := s.repo.UserInstalledSkillByID(instanceID); err != nil {
			return nil, true, err
		} else if other != nil {
			return nil, true, kernel.Forbidden("无权编辑该技能")
		}
		return nil, false, nil
	}
	if instance.SourceType == "github" {
		appErr := kernel.NewAppError(409, "GitHub 技能以仓库内容为准，请在仓库修改后执行同步")
		appErr.Reason = kernel.ReasonFailedPrecondition
		return nil, true, appErr
	}
	if len(req.Operations) == 0 {
		return nil, true, kernel.BadAuthRequest("没有需要保存的文件修改")
	}
	if len(req.Operations) > maxSkillEditOperations {
		return nil, true, kernel.BadAuthRequest("单次最多提交 256 个文件操作")
	}
	if req.BaseVersionID == "" || req.BaseVersionID != instance.ContentID {
		return nil, true, skillVersionConflict(instance.ContentID)
	}
	snapshot, err := s.instanceSnapshot(instance)
	if err != nil {
		return nil, true, err
	}
	next, err := readSkillContentEntries(snapshot.content)
	if err != nil {
		return nil, true, err
	}
	for _, operation := range req.Operations {
		if err := applySkillFileOperation(next, operation); err != nil {
			return nil, true, err
		}
	}
	if err := validateSkillPackageTree(next); err != nil {
		return nil, true, err
	}
	metadata := parseSkillPackageMetadata(next["SKILL.md"])
	if metadata.Name == "" {
		metadata.Name = truncateSkillMetadata(instance.Name, 80)
	}
	if metadata.Description == "" {
		metadata.Description = truncateSkillMetadata(instance.Description, 500)
	}
	archive, err := finalizeSkillArchive(next, metadata)
	if err != nil {
		return nil, true, err
	}
	if archive.ContentHash == snapshot.version.ContentHash {
		item, err := s.userSkillItem(userID, instance, true)
		if err != nil {
			return nil, true, err
		}
		return &SkillPackageEditResult{Skill: item, Files: skillFileItems(snapshot), Changed: false}, true, nil
	}
	contentID := kernel.NewID()
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		return nil, true, err
	}
	now := time.Now()
	content := &model.SkillContent{ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data, CreatedAt: now}
	files := s.contentFileRecords(contentID, archive.Files)
	sourceType := strings.TrimSpace(instance.SourceType)
	if sourceType == "" {
		sourceType = "markdown"
	} else if sourceType == "markdown" && len(archive.Files) > 1 {
		sourceType = "zip"
	}
	fields := map[string]any{
		"content_id": contentID, "content_hash": archive.ContentHash,
		"version_number": gorm.Expr("version_number + 1"), "version_label": editedSkillVersionLabel(),
		"follow_updates": false, "follow_metadata": false, "source_type": sourceType,
		"file_count": len(archive.Files), "total_bytes": archive.TotalBytes, "updated_at": now,
	}
	if err := s.repo.ReplaceUserSkillContent(instance.ID, instance.ContentID, fields, content, files); err != nil {
		return nil, true, err
	}
	updated, err := s.repo.UserInstalledSkill(userID, instance.ID)
	if err != nil || updated == nil {
		return nil, true, err
	}
	item, err := s.userSkillItem(userID, updated, true)
	if err != nil {
		return nil, true, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	fileItems := make([]SkillPackageFileItem, 0, len(files))
	for _, file := range files {
		fileItems = append(fileItems, SkillPackageFileItem{Path: file.Path, Kind: file.Kind, MimeType: file.MimeType, Size: file.Size, SHA256: file.SHA256})
	}
	return &SkillPackageEditResult{Skill: item, Files: fileItems, Changed: true}, true, nil
}
