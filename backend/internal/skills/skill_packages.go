package skills

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

const (
	maxSkillPackageBytes       = 20 << 20
	maxSkillFileBytes          = 8 << 20
	maxSkillPackageFiles       = 512
	maxSkillPreviewBytes       = 512 << 10
	SkillPackageUploadMaxBytes = maxSkillPackageBytes + (1 << 20)
)

type SkillInstallRequest struct {
	Name        string
	Description string
	Tag         string
	IsPrivate   bool
}

type SkillGitHubInstallRequest struct {
	URL        string `json:"url"`
	Ref        string `json:"ref"`
	Subdir     string `json:"subdir"`
	Tag        string `json:"tag"`
	IsPrivate  bool   `json:"isPrivate"`
	AutoUpdate bool   `json:"autoUpdate"`
}

type SkillPackageFileItem struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

type SkillPackageFileContent struct {
	File    SkillPackageFileItem `json:"file"`
	Content string               `json:"content"`
	Binary  bool                 `json:"binary"`
}

type SkillPackageBundleFile struct {
	Path          string `json:"path"`
	MimeType      string `json:"mimeType"`
	ContentBase64 string `json:"contentBase64"`
}

type SkillPackageBundle struct {
	SkillID     string                   `json:"skillId"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	VersionID   string                   `json:"versionId"`
	Version     string                   `json:"version"`
	ContentHash string                   `json:"contentHash"`
	Files       []SkillPackageBundleFile `json:"files"`
}

type SkillFileSearchResult struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type skillPackageMetadata struct {
	Name        string
	Description string
	Version     string
}

type skillPackageArchive struct {
	Files       map[string][]byte
	Metadata    skillPackageMetadata
	ContentHash string
	TotalBytes  int64
}

type githubSkillSpec struct {
	Owner  string
	Repo   string
	Ref    string
	Subdir string
}

func (s *Service) InstallSkillUpload(userID string, sourceType string, header *multipart.FileHeader, req SkillInstallRequest) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	sourceType = strings.ToLower(strings.TrimSpace(sourceType))
	if sourceType == "" && header != nil {
		switch strings.ToLower(path.Ext(header.Filename)) {
		case ".md", ".markdown":
			sourceType = "markdown"
		case ".zip":
			sourceType = "zip"
		}
	}
	if sourceType != "markdown" && sourceType != "zip" {
		return nil, kernel.BadAuthRequest("技能文件仅支持 Markdown 或 ZIP")
	}
	if header == nil || header.Size <= 0 || header.Size > maxSkillPackageBytes {
		return nil, kernel.BadAuthRequest("技能文件大小必须在 1B-20MB 之间")
	}
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSkillPackageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSkillPackageBytes {
		return nil, kernel.BadAuthRequest("技能文件不能超过 20MB")
	}
	var archive skillPackageArchive
	if sourceType == "markdown" {
		archive, err = archiveFromMarkdown(data, req.Name, req.Description)
	} else {
		archive, err = archiveFromZip(data, "")
	}
	if err != nil {
		return nil, err
	}
	instance, err := s.createPrivateUserSkill(userID, archive, sourceType, "", "", "", "", false, req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, instance, true)
}

func (s *Service) InstallGitHubSkill(userID string, req SkillGitHubInstallRequest) (*SkillItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	spec, err := parseGitHubSkillURL(req.URL, req.Ref, req.Subdir)
	if err != nil {
		return nil, err
	}
	archive, commit, canonicalURL, resolvedRef, err := fetchGitHubSkillArchive(context.Background(), spec)
	if err != nil {
		var appErr *kernel.AppError
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, kernel.WrapAppError(http.StatusBadGateway, "GitHub 技能读取失败，请检查仓库地址和网络", err)
	}
	instance, err := s.createPrivateUserSkill(userID, archive, "github", canonicalURL, resolvedRef, spec.Subdir, commit, req.AutoUpdate, "", "")
	if err != nil {
		return nil, err
	}
	return s.userSkillItem(userID, instance, true)
}

func (s *Service) SyncGitHubSkill(userID string, skillID string) (*SkillItem, error) {
	if instance, err := s.repo.UserInstalledSkill(userID, skillID); err != nil {
		return nil, err
	} else if instance != nil {
		return s.syncUserGitHubSkill(userID, instance)
	}
	skill, err := s.ownedSkill(userID, skillID)
	if err != nil {
		return nil, err
	}
	if skill.SourceType != "github" {
		return nil, kernel.BadAuthRequest("只有 GitHub 技能可以同步")
	}
	if err := s.syncGitHubSkill(skill); err != nil {
		var appErr *kernel.AppError
		if errors.As(err, &appErr) {
			return nil, err
		}
		return nil, kernel.WrapAppError(http.StatusBadGateway, "GitHub 技能同步失败，请稍后重试", err)
	}
	return s.SkillDetail(userID, skill.ID)
}

func (s *Service) StartSyncWorker(ctx context.Context) {
	s.runWorkerLoop(func(ctx context.Context) {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				skills, err := s.repo.AutoUpdatingGitHubSkills()
				if err != nil {
					continue
				}
				for index := range skills {
					_ = s.syncGitHubSkillWithContext(ctx, &skills[index])
				}
			}
		}
	})
}

func (s *Service) syncGitHubSkill(skill *model.Skill) error {
	return s.syncGitHubSkillWithContext(context.Background(), skill)
}

func (s *Service) syncGitHubSkillWithContext(ctx context.Context, skill *model.Skill) error {
	if skill == nil {
		return kernel.BadAuthRequest("技能不存在")
	}
	if err := s.RequireAdministrator(skill.OwnerID); err != nil {
		return err
	}
	now := time.Now()
	spec, err := parseGitHubSkillURL(skill.SourceURL, skill.SourceRef, skill.SourceSubdir)
	if err != nil {
		skill.SyncStatus = "failed"
		skill.SyncError = err.Error()
		skill.LastCheckedAt = &now
		_ = s.repo.SaveSkill(skill)
		return err
	}
	archive, commit, canonicalURL, resolvedRef, err := fetchGitHubSkillArchive(ctx, spec)
	skill.LastCheckedAt = &now
	if err != nil {
		skill.SyncStatus = "failed"
		skill.SyncError = err.Error()
		_ = s.repo.SaveSkill(skill)
		return err
	}
	if commit == skill.SourceCommit {
		skill.SyncStatus = "synced"
		skill.SyncError = ""
		return s.repo.SaveSkill(skill)
	}
	// 管理员改过基本信息后，以后台填写的名称和简介为准；文件内容仍以仓库为准。
	if skill.AdminInfoEditedAt == nil {
		skill.Name = archive.Metadata.Name
		skill.Description = archive.Metadata.Description
	}
	if err := s.addSkillArchiveContent(skill, archive, "github", canonicalURL, resolvedRef, spec.Subdir, commit, skill.AutoUpdate); err != nil {
		skill.SyncStatus = "failed"
		skill.SyncError = err.Error()
		_ = s.repo.SaveSkill(skill)
		return err
	}
	return nil
}

func (s *Service) addSkillArchiveContent(skill *model.Skill, archive skillPackageArchive, sourceType string, sourceURL string, sourceRef string, sourceSubdir string, sourceCommit string, autoUpdate bool) error {
	versionLabel := archive.Metadata.Version
	if versionLabel == "" {
		versionLabel = "1"
		if sourceCommit != "" {
			versionLabel = shortCommit(sourceCommit)
		} else if strings.TrimSpace(skill.ContentID) != "" {
			versionLabel = editedSkillVersionLabel()
		}
	}
	content, files, err := s.contentRecord(archive)
	if err != nil {
		return err
	}
	previous := skill.ContentID
	applySkillArchiveContent(skill, content, versionLabel, archive, sourceType, sourceURL, sourceRef, sourceSubdir, sourceCommit, autoUpdate)
	return s.repo.AddSkillContent(skill, content, files, previous)
}

func editedSkillVersionLabel() string {
	return time.Now().UTC().Format("20060102-150405")
}

// applySkillArchiveContent 把内容快照元数据同步到技能行；调用方负责在同一事务里写入内容行和文件索引。
func applySkillArchiveContent(skill *model.Skill, content *model.SkillContent, versionLabel string, archive skillPackageArchive, sourceType string, sourceURL string, sourceRef string, sourceSubdir string, sourceCommit string, autoUpdate bool) {
	now := time.Now()
	skill.ContentID = content.ID
	skill.VersionLabel = versionLabel
	skill.ContentHash = archive.ContentHash
	skill.FileCount = len(archive.Files)
	skill.TotalBytes = archive.TotalBytes
	skill.SourceType = sourceType
	skill.SourceURL = sourceURL
	skill.SourceRef = sourceRef
	skill.SourceSubdir = sourceSubdir
	skill.SourceCommit = sourceCommit
	skill.SyncStatus = "synced"
	skill.SyncError = ""
	skill.AutoUpdate = autoUpdate
	skill.LastCheckedAt = &now
	skill.LastSyncedAt = &now
}

func normalizeSkillPath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") {
		return "", kernel.BadAuthRequest("技能文件路径无效")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") || strings.HasPrefix(clean, ".git/") || clean == ".git" {
		return "", kernel.BadAuthRequest("技能文件路径越界或包含禁止目录")
	}
	if utf8.RuneCountInString(clean) > 1000 {
		return "", kernel.BadAuthRequest("技能文件路径过长")
	}
	return clean, nil
}

func skillFileMime(filePath string, content []byte) string {
	if filePath == "SKILL.md" || strings.HasSuffix(strings.ToLower(filePath), ".md") {
		return "text/markdown; charset=utf-8"
	}
	if detected := mime.TypeByExtension(strings.ToLower(path.Ext(filePath))); detected != "" {
		return detected
	}
	return http.DetectContentType(content)
}

func skillFileKind(filePath string) string {
	extension := strings.ToLower(path.Ext(filePath))
	switch extension {
	case ".md", ".mdx":
		return "markdown"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg":
		return "image"
	case ".mp4", ".webm", ".mov":
		return "video"
	case ".mp3", ".wav", ".m4a", ".ogg":
		return "audio"
	case ".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".sh", ".json", ".yaml", ".yml", ".toml", ".html", ".css":
		return "code"
	case ".txt", ".csv":
		return "text"
	default:
		return "binary"
	}
}

func isPreviewText(mimeType string, filePath string) bool {
	return strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "json") || strings.Contains(mimeType, "yaml") || strings.Contains(mimeType, "toml") || skillFileKind(filePath) == "code"
}

func skillFileItem(file skillPackageFile) SkillPackageFileItem {
	return SkillPackageFileItem{Path: file.Path, Kind: file.Kind, MimeType: file.MimeType, Size: file.Size, SHA256: file.SHA256}
}

func sortedSkillPaths(files map[string][]byte) []string {
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths
}

func shortCommit(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
