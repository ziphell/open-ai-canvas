package repository

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SkillContentsMissingData 返回仍被技能或用户实例引用、但尚未导入 BLOB 的内容占位行。
func (r *Repository) SkillContentsMissingData(limit int) ([]model.SkillContent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	var rows []model.SkillContent
	err := r.db.Select("id, format, size, sha256, created_at").
		Where("data IS NULL").
		Where("EXISTS (SELECT 1 FROM skills s WHERE s.content_id = skill_contents.id) OR EXISTS (SELECT 1 FROM user_installed_skills i WHERE i.content_id = skill_contents.id)").
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// SaveSkillContentArchive 在一个事务里写入内容 BLOB 和文件索引。
func (r *Repository) SaveSkillContentArchive(contentID string, data []byte, size int64, sha256 string, files []model.SkillContentFile) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		row := model.SkillContent{ID: contentID, Format: "zip", Size: size, SHA256: sha256, Data: data}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"format", "size", "sha256", "data"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.SkillContentFile{}, "content_id = ?", contentID).Error; err != nil {
			return err
		}
		if len(files) == 0 {
			return nil
		}
		return tx.CreateInBatches(&files, 100).Error
	})
}

// SkillContentWithData 读取内容快照，包含 BLOB；只给正文读取路径使用。
func (r *Repository) SkillContentWithData(id string) (*model.SkillContent, error) {
	var content model.SkillContent
	if err := r.db.First(&content, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &content, nil
}

// SkillContentFiles 返回某个内容快照的文件索引。
func (r *Repository) SkillContentFiles(contentID string) ([]model.SkillContentFile, error) {
	var files []model.SkillContentFile
	err := r.db.Where("content_id = ?", contentID).Order("path ASC").Find(&files).Error
	return files, err
}

// SkillContentFilePaths 返回多个内容快照的可读文本文件路径。
func (r *Repository) SkillContentFilePaths(contentIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(contentIDs))
	if len(contentIDs) == 0 {
		return result, nil
	}
	var rows []model.SkillContentFile
	if err := r.db.Where("content_id IN ?", contentIDs).Order("path ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ContentID] = append(result[row.ContentID], row.Path)
	}
	return result, nil
}

// SyncUserInstalledSkillContentMetadata 补齐共享同一内容快照的实例元数据。
func (r *Repository) SyncUserInstalledSkillContentMetadata(contentID string, contentHash string, fileCount int, totalBytes int64) error {
	if contentID == "" {
		return nil
	}
	return r.db.Model(&model.UserInstalledSkill{}).Where("content_id = ?", contentID).
		Updates(map[string]any{"content_hash": contentHash, "file_count": fileCount, "total_bytes": totalBytes}).Error
}

// SkillContentExists 判断内容快照是否已导入 BLOB。
func (r *Repository) SkillContentExists(id string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.SkillContent{}).Where("id = ? AND data IS NOT NULL", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
