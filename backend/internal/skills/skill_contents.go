package skills

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

// EnsureSkillContents 把旧 skill-packages 文件或 legacy instruction 导入 skill_contents，
// 并建立按内容 ID 索引的文件清单。已经导入的内容不会重复写入。
func (s *Service) EnsureSkillContents() error {
	rows, err := s.repo.SkillContentsMissingData(1000)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	imported, failed := 0, 0
	for _, row := range rows {
		archive, err := s.loadArchiveForContent(row.ID)
		if err != nil {
			log.Printf("技能内容 %s 暂时无法导入：%v", row.ID, err)
			failed++
			continue
		}
		data, err := encodeSkillArchive(archive.Files)
		if err != nil {
			log.Printf("技能内容 %s 重新编码失败：%v", row.ID, err)
			failed++
			continue
		}
		files := s.contentFileRecords(row.ID, archive.Files)
		if err := s.repo.SaveSkillContentArchive(row.ID, data, int64(len(data)), archive.ContentHash, files); err != nil {
			log.Printf("技能内容 %s 写入失败：%v", row.ID, err)
			failed++
			continue
		}
		if row.SHA256 != "" && row.SHA256 != archive.ContentHash {
			log.Printf("技能内容 %s 哈希与旧记录不一致：old=%s new=%s", row.ID, row.SHA256, archive.ContentHash)
		}
		if skill, skillErr := s.repo.SkillByContentID(row.ID); skillErr == nil {
			if err := s.syncSkillContentMetadata(skill, archive.ContentHash, len(files), archive.TotalBytes); err != nil {
				log.Printf("技能 %s 内容元数据同步失败：%v", skill.ID, err)
				failed++
				continue
			}
		}
		if err := s.repo.SyncUserInstalledSkillContentMetadata(row.ID, archive.ContentHash, len(files), archive.TotalBytes); err != nil {
			log.Printf("技能内容 %s 的安装快照同步失败：%v", row.ID, err)
			failed++
			continue
		}
		imported++
	}
	if imported > 0 || failed > 0 {
		log.Printf("技能内容导入完成：imported=%d failed=%d", imported, failed)
	}
	return nil
}

// syncSkillContentMetadata 只补齐内容指针指向当前内容的技能行元数据，不改名称、简介或上架状态。
func (s *Service) syncSkillContentMetadata(skill *model.Skill, contentHash string, fileCount int, totalBytes int64) error {
	if skill == nil || (skill.ContentHash == contentHash && skill.FileCount == fileCount && skill.TotalBytes == totalBytes) {
		return nil
	}
	return s.repo.UpdateSkillFields(skill.ID, map[string]any{
		"content_hash": contentHash, "file_count": fileCount, "total_bytes": totalBytes,
	})
}

// contentSnapshot 用内容快照构造文件索引；内容尚未导入时返回 false。
func (s *Service) contentSnapshot(skill *model.Skill) (*skillPackageSnapshot, bool) {
	if skill == nil {
		return nil, false
	}
	contentID := strings.TrimSpace(skill.ContentID)
	if contentID == "" {
		contentID = skill.ID
	}
	content, err := s.repo.SkillContentWithData(contentID)
	if err != nil || len(content.Data) == 0 {
		return nil, false
	}
	if skill.ContentHash != "" && content.SHA256 != "" && content.SHA256 != skill.ContentHash {
		return nil, false
	}
	rows, err := s.repo.SkillContentFiles(contentID)
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	files := make([]skillPackageFile, 0, len(rows))
	var totalBytes int64
	for _, row := range rows {
		files = append(files, skillPackageFile{
			Path: row.Path, Kind: row.Kind, MimeType: row.MimeType, Size: row.Size, SHA256: row.SHA256,
		})
		totalBytes += row.Size
	}
	version := &skillPackageVersion{
		ID: contentID, VersionLabel: skill.VersionLabel, ContentHash: content.SHA256, EntryPath: "SKILL.md",
		FileCount: len(files), TotalBytes: totalBytes, CreatedAt: content.CreatedAt,
	}
	return &skillPackageSnapshot{skill: skill, version: version, files: files, content: content}, true
}

// loadArchiveForContent 在内容 BLOB 缺失时从旧磁盘 ZIP 或 instruction 重建。
// 旧目录结构固定为 skill-packages/<skillID>/<contentID>.zip。
func (s *Service) loadArchiveForContent(contentID string) (skillPackageArchive, error) {
	if skill, err := s.repo.SkillByContentID(contentID); err == nil {
		absolute := filepath.Join(s.dataDir, "skill-packages", filepath.FromSlash(skill.ID), contentID+".zip")
		if data, readErr := os.ReadFile(absolute); readErr == nil {
			if archive, parseErr := archiveFromZip(data, ""); parseErr == nil {
				return archive, nil
			}
		}
		if strings.TrimSpace(skill.Instruction) != "" {
			return archiveFromMarkdown([]byte(skill.Instruction), truncateSkillMetadata(skill.Name, 80), truncateSkillMetadata(skill.Description, 500))
		}
	}
	// 早期只有 instruction 的技能没有版本 ID，内容 ID 就是技能 ID。
	if skill, err := s.repo.SkillAnyStatus(contentID); err == nil && strings.TrimSpace(skill.Instruction) != "" {
		return archiveFromMarkdown([]byte(skill.Instruction), truncateSkillMetadata(skill.Name, 80), truncateSkillMetadata(skill.Description, 500))
	}
	return skillPackageArchive{}, errors.New("缺少可导入的技能文件或正文")
}

// contentRecord 把技能包编码成不可变内容行和文件索引。
func (s *Service) contentRecord(archive skillPackageArchive) (*model.SkillContent, []model.SkillContentFile, error) {
	contentID := kernel.NewID()
	data, err := encodeSkillArchive(archive.Files)
	if err != nil {
		return nil, nil, err
	}
	return &model.SkillContent{
		ID: contentID, Format: "zip", Size: int64(len(data)), SHA256: archive.ContentHash, Data: data, CreatedAt: time.Now(),
	}, s.contentFileRecords(contentID, archive.Files), nil
}
