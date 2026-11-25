package repository

import (
	"errors"
	"strings"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 内置技能使用稳定的外部技能 ID 幂等更新，用户安装关系保留在独立表。
// columns 是冲突时覆盖的列，由调用方按管理员是否改过该技能决定；status 不应出现在其中，下架状态只由管理员修改。
func (r *Repository) UpsertBuiltinSkills(skills []model.Skill, columns []string) error {
	if len(skills) == 0 {
		return errors.New("builtin skills are empty")
	}
	if len(columns) == 0 {
		return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&skills).Error
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&skills).Error
}

func (r *Repository) Skill(id string) (*model.Skill, error) {
	var skill model.Skill
	if err := r.db.First(&skill, "id = ? AND status = ?", id, 1).Error; err != nil {
		return nil, err
	}
	return &skill, nil
}

// SkillAnyStatus 读取技能而不过滤上架状态，只给管理员入口和内置技能同步使用。
func (r *Repository) SkillAnyStatus(id string) (*model.Skill, error) {
	var skill model.Skill
	if err := r.db.First(&skill, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &skill, nil
}

func (r *Repository) SkillsByIDsAnyStatus(ids []string) ([]model.Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var skills []model.Skill
	err := r.db.Where("id IN ?", ids).Find(&skills).Error
	return skills, err
}

type AdminSkillListFilter struct {
	Keyword    string
	Status     string
	SourceType string
	Tag        string
	Limit      int
	Offset     int
}

// AdminSkills 列出全部技能（含私有和已下架），按最近更新排序。
func (r *Repository) AdminSkills(filter AdminSkillListFilter) ([]model.Skill, int64, error) {
	query := r.db.Model(&model.Skill{})
	switch filter.Status {
	case "enabled":
		query = query.Where("skills.status = ?", 1)
	case "disabled":
		query = query.Where("COALESCE(skills.status, 0) <> ?", 1)
	}
	switch filter.SourceType {
	case "":
	case "markdown":
		query = query.Where("skills.source_type IN ?", []string{"markdown", ""})
	default:
		query = query.Where("skills.source_type = ?", filter.SourceType)
	}
	if filter.Tag != "" {
		query = query.Where("skills.tag = ?", filter.Tag)
	}
	if filter.Keyword != "" {
		pattern := "%" + strings.ToLower(filter.Keyword) + "%"
		query = query.Joins("LEFT JOIN users skill_owners ON skill_owners.id = skills.owner_id").
			Where("skills.id = ? OR lower(skills.name) LIKE ? OR lower(skills.description) LIKE ? OR lower(skills.author_name) LIKE ? OR lower(skill_owners.display_name) LIKE ? OR lower(skill_owners.username) LIKE ? OR lower(skills.source_url) LIKE ?",
				filter.Keyword, pattern, pattern, pattern, pattern, pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var skills []model.Skill
	err := query.Select("skills.*").Order("skills.updated_at desc, skills.id asc").Limit(filter.Limit).Offset(filter.Offset).Find(&skills).Error
	return skills, total, err
}

// UpdateSkillStatus 只改上架状态，不刷新 updated_at，避免下架 / 上架改变技能广场的“最近更新”排序。
func (r *Repository) UpdateSkillStatus(id string, status int) error {
	return r.db.Model(&model.Skill{}).Where("id = ?", id).UpdateColumn("status", status).Error
}

// UpdateSkillFields 按列更新技能行，避免整行保存覆盖并发写入的状态或版本字段。
func (r *Repository) UpdateSkillFields(id string, fields map[string]any) error {
	return r.db.Model(&model.Skill{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repository) DeleteSkill(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var locked model.Skill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.SkillCurationAssignment{}, "skill_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.SkillCurationRootAssignment{}, "skill_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.UserSkillFavorite{}, "skill_id = ?", id).Error; err != nil {
			return err
		}
		// 已安装副本保留内容快照，只解除与公共目录的关联并停止跟随。
		if err := tx.Model(&model.UserInstalledSkill{}).Where("source_skill_id = ?", id).
			Updates(map[string]any{"source_skill_id": nil, "follow_updates": false, "follow_metadata": false}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.MarketSkillListing{}, "skill_id = ?", id).Error; err != nil {
			return err
		}
		contentID := locked.ContentID
		if err := tx.Delete(&model.Skill{}, "id = ?", id).Error; err != nil {
			return err
		}
		return deleteUnreferencedSkillContent(tx, contentID)
	})
}

func (r *Repository) SkillOwners(ownerIDs []string) (map[string]model.User, error) {
	owners := make(map[string]model.User, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return owners, nil
	}
	var users []model.User
	if err := r.db.Find(&users, "id IN ?", ownerIDs).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		owners[user.ID] = user
	}
	return owners, nil
}

func (r *Repository) SkillOwnerAvatars(ownerIDs []string) (map[string]string, error) {
	avatars := make(map[string]string, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return avatars, nil
	}
	var identities []model.UserIdentity
	if err := r.db.Where("user_id IN ? AND avatar_url <> ''", ownerIDs).Order("updated_at desc").Find(&identities).Error; err != nil {
		return nil, err
	}
	for _, identity := range identities {
		if avatars[identity.UserID] == "" {
			avatars[identity.UserID] = identity.AvatarURL
		}
	}
	return avatars, nil
}
