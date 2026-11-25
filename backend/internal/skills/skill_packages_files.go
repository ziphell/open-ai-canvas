// 技能包内文件的浏览、读取、打包下载与全文检索。

package skills

import (
	"encoding/base64"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

// skillPackageFile 是内容快照中的单个文件；正文统一从 skill_contents 读取。
type skillPackageFile struct {
	Path     string
	Kind     string
	MimeType string
	Size     int64
	SHA256   string
}

// skillPackageVersion 描述当前内容快照的元数据，字段与接口里的版本展示保持一致。
type skillPackageVersion struct {
	ID           string
	VersionLabel string
	ContentHash  string
	EntryPath    string
	FileCount    int
	TotalBytes   int64
	CreatedAt    time.Time
}

type skillPackageSnapshot struct {
	skill   *model.Skill
	version *skillPackageVersion
	files   []skillPackageFile
	content *model.SkillContent
}

func (s *Service) SkillPackageFiles(userID string, skillID string) ([]SkillPackageFileItem, error) {
	snapshot, err := s.loadSkillPackageSnapshot(userID, skillID)
	if err != nil {
		return nil, err
	}
	return skillFileItems(snapshot), nil
}

func (s *Service) SkillPackageFile(userID string, skillID string, filePath string) (*SkillPackageFileContent, error) {
	snapshot, err := s.loadSkillPackageSnapshot(userID, skillID)
	if err != nil {
		return nil, err
	}
	return s.skillPackageFileContent(snapshot, filePath)
}

func (s *Service) skillPackageFileContent(snapshot *skillPackageSnapshot, filePath string) (*SkillPackageFileContent, error) {
	file, content, err := s.readSkillPackageSnapshotFile(snapshot, filePath)
	if err != nil {
		return nil, err
	}
	binary := !isPreviewText(file.MimeType, file.Path)
	if binary {
		return &SkillPackageFileContent{File: skillFileItem(*file), Binary: true}, nil
	}
	if len(content) > maxSkillPreviewBytes {
		return nil, kernel.BadAuthRequest("文件超过 512KB，请下载后查看")
	}
	return &SkillPackageFileContent{File: skillFileItem(*file), Content: string(content)}, nil
}

func (s *Service) SkillPackageRawFile(userID string, skillID string, filePath string) ([]byte, string, string, error) {
	snapshot, err := s.loadSkillPackageSnapshot(userID, skillID)
	if err != nil {
		return nil, "", "", err
	}
	return s.skillPackageRawFile(snapshot, filePath)
}

func (s *Service) skillPackageRawFile(snapshot *skillPackageSnapshot, filePath string) ([]byte, string, string, error) {
	file, content, err := s.readSkillPackageSnapshotFile(snapshot, filePath)
	if err != nil {
		return nil, "", "", err
	}
	return content, file.MimeType, path.Base(file.Path), nil
}

func (s *Service) SkillPackageBundle(userID string, skillID string) (*SkillPackageBundle, error) {
	snapshot, err := s.loadSkillPackageSnapshot(userID, skillID)
	if err != nil {
		return nil, err
	}
	bundle := &SkillPackageBundle{SkillID: snapshot.skill.ID, Name: snapshot.skill.Name, Description: snapshot.skill.Description, VersionID: snapshot.version.ID, Version: snapshot.version.VersionLabel, ContentHash: snapshot.version.ContentHash, Files: make([]SkillPackageBundleFile, 0, len(snapshot.files))}
	for _, file := range snapshot.files {
		content, err := s.readSkillPackageSnapshotEntry(snapshot, file.Path)
		if err != nil {
			return nil, err
		}
		bundle.Files = append(bundle.Files, SkillPackageBundleFile{Path: file.Path, MimeType: file.MimeType, ContentBase64: base64.StdEncoding.EncodeToString(content)})
	}
	return bundle, nil
}

func (s *Service) SearchSkillPackage(userID string, skillID string, query string) ([]SkillFileSearchResult, error) {
	snapshot, err := s.loadSkillPackageSnapshot(userID, skillID)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 120 {
		return nil, kernel.BadAuthRequest("搜索关键词必须为 1-120 个字符")
	}
	needle := strings.ToLower(query)
	results := make([]SkillFileSearchResult, 0, 20)
	for _, file := range snapshot.files {
		if len(results) >= 50 || !isPreviewText(file.MimeType, file.Path) || file.Size > maxSkillPreviewBytes {
			continue
		}
		content, err := s.readSkillPackageSnapshotEntry(snapshot, file.Path)
		if err != nil {
			return nil, err
		}
		for lineIndex, line := range strings.Split(string(content), "\n") {
			if strings.Contains(strings.ToLower(line), needle) {
				results = append(results, SkillFileSearchResult{Path: file.Path, Line: lineIndex + 1, Snippet: kernel.TruncateRunes(strings.TrimSpace(line), 240)})
				if len(results) >= 50 {
					break
				}
			}
		}
	}
	return results, nil
}

func (s *Service) readSkillPackageSnapshotFile(snapshot *skillPackageSnapshot, filePath string) (*skillPackageFile, []byte, error) {
	filePath, err := normalizeSkillPath(filePath)
	if err != nil {
		return nil, nil, err
	}
	for index := range snapshot.files {
		if snapshot.files[index].Path != filePath {
			continue
		}
		content, err := s.readSkillPackageSnapshotEntry(snapshot, filePath)
		return &snapshot.files[index], content, err
	}
	return nil, nil, kernel.BadAuthRequest("技能文件不存在")
}

func (s *Service) loadSkillPackageSnapshot(userID string, skillID string) (*skillPackageSnapshot, error) {
	if instance, err := s.repo.UserInstalledSkill(userID, skillID); err != nil {
		return nil, err
	} else if instance != nil {
		return s.instanceSnapshot(instance)
	}
	if skill, err := s.visibleCatalogSkill(userID, skillID); err == nil {
		return s.skillPackageSnapshot(skill)
	}
	skill, err := s.visibleSkill(userID, skillID)
	if err != nil {
		return nil, err
	}
	return s.skillPackageSnapshot(skill)
}

// skillPackageSnapshot 读取技能当前内容快照；调用方负责先完成可见性或管理员校验。
func (s *Service) skillPackageSnapshot(skill *model.Skill) (*skillPackageSnapshot, error) {
	if snapshot, ok := s.contentSnapshot(skill); ok {
		return snapshot, nil
	}
	return nil, fmt.Errorf("技能 %s 内容尚未就绪", skill.ID)
}

func (s *Service) readSkillPackageSnapshotEntry(snapshot *skillPackageSnapshot, filePath string) ([]byte, error) {
	if snapshot.content == nil {
		return nil, kernel.BadAuthRequest("技能内容尚未就绪")
	}
	return readSkillContentEntry(snapshot.content, filePath)
}
