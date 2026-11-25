package repository

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

// CreateSkillWithContent 在一个事务里创建公共目录技能行、内容快照和文件索引。
func (r *Repository) CreateSkillWithContent(skill *model.Skill, content *model.SkillContent, files []model.SkillContentFile) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(skill).Error; err != nil {
			return err
		}
		return createSkillContent(tx, content, files)
	})
}

// AddSkillContent 写入新的内容快照并切换技能的内容指针。
// previousContentID 指向切换前的内容，失去全部引用时会被清理。
func (r *Repository) AddSkillContent(skill *model.Skill, content *model.SkillContent, files []model.SkillContentFile, previousContentID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := createSkillContent(tx, content, files); err != nil {
			return err
		}
		if err := tx.Omit(skillStatusColumn).Save(skill).Error; err != nil {
			return err
		}
		return deleteUnreferencedSkillContent(tx, previousContentID)
	})
}

// AddSkillContentIfCurrent 以 content_id 做乐观锁：只有技能当前内容仍是 baseContentID 时
// 才写入新内容、文件索引和技能行；返回 false 表示已被并发修改，事务内不做任何写入。
func (r *Repository) AddSkillContentIfCurrent(skill *model.Skill, content *model.SkillContent, files []model.SkillContentFile, baseContentID string, previousContentID string) (bool, error) {
	applied := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Skill{}).Where("id = ? AND content_id = ?", skill.ID, baseContentID).Update("content_id", content.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := createSkillContent(tx, content, files); err != nil {
			return err
		}
		if err := tx.Omit(skillStatusColumn).Save(skill).Error; err != nil {
			return err
		}
		applied = true
		return deleteUnreferencedSkillContent(tx, previousContentID)
	})
	return applied, err
}

func createSkillContent(tx *gorm.DB, content *model.SkillContent, files []model.SkillContentFile) error {
	if err := tx.Create(content).Error; err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return tx.CreateInBatches(&files, 100).Error
}

// deleteUnreferencedSkillContent 只删除不再被公共目录或用户实例引用的内容快照。
func deleteUnreferencedSkillContent(tx *gorm.DB, contentID string) error {
	if contentID == "" {
		return nil
	}
	var references int64
	if err := tx.Model(&model.Skill{}).Where("content_id = ?", contentID).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return nil
	}
	if err := tx.Model(&model.UserInstalledSkill{}).Where("content_id = ?", contentID).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return nil
	}
	if err := tx.Delete(&model.SkillContentFile{}, "content_id = ?", contentID).Error; err != nil {
		return err
	}
	return tx.Delete(&model.SkillContent{}, "id = ?", contentID).Error
}

// SkillByContentID 按内容指针读取公共目录技能，用于旧内容导入时定位磁盘 ZIP。
func (r *Repository) SkillByContentID(contentID string) (*model.Skill, error) {
	var skill model.Skill
	if err := r.db.First(&skill, "content_id = ?", contentID).Error; err != nil {
		return nil, err
	}
	return &skill, nil
}

func (r *Repository) AutoUpdatingGitHubSkills() ([]model.Skill, error) {
	var skills []model.Skill
	err := r.db.Where("status = ? AND source_type = ? AND auto_update = ?", 1, "github", true).Find(&skills).Error
	return skills, err
}

// skillStatusColumn 只由 UpdateSkillStatus 修改：同步、编辑等整行保存可能持有下架前读出的旧行，不能把下架改回上架。
const skillStatusColumn = "status"

func (r *Repository) SaveSkill(skill *model.Skill) error {
	return r.db.Omit(skillStatusColumn).Save(skill).Error
}
