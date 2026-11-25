package repository

import (
	"errors"
	"strings"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserInstalledSkillFilter struct {
	UserID               string
	CreatedOnly          bool
	Search               string
	LibraryCategoryID    string
	LibraryUncategorized bool
	Limit                int
	Offset               int
}

func (r *Repository) UserInstalledSkills(filter UserInstalledSkillFilter) ([]model.UserInstalledSkill, int64, error) {
	query := r.db.Model(&model.UserInstalledSkill{}).Where("user_id = ?", filter.UserID)
	if filter.CreatedOnly {
		query = query.Where("source_skill_id IS NULL")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		query = query.Where("(lower(name) LIKE ? OR lower(description) LIKE ?)", pattern, pattern)
	}
	if filter.LibraryCategoryID != "" {
		query = query.Where("library_category_id = ?", filter.LibraryCategoryID)
	} else if filter.LibraryUncategorized {
		query = query.Where("COALESCE(library_category_id, '') = ''")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.UserInstalledSkill
	err := query.Order("updated_at DESC, id ASC").Limit(filter.Limit).Offset(filter.Offset).Find(&rows).Error
	return rows, total, err
}

func (r *Repository) UserInstalledSkill(userID, id string) (*model.UserInstalledSkill, error) {
	var row model.UserInstalledSkill
	err := r.db.First(&row, "id = ? AND user_id = ?", id, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) UserInstalledSkillByID(id string) (*model.UserInstalledSkill, error) {
	var row model.UserInstalledSkill
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) UserInstalledSkillsAll(userID string) ([]model.UserInstalledSkill, error) {
	var rows []model.UserInstalledSkill
	err := r.db.Where("user_id = ?", userID).Order("updated_at DESC, id ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) UserInstalledSkillsByIDs(userID string, ids []string) ([]model.UserInstalledSkill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.UserInstalledSkill
	err := r.db.Where("user_id = ? AND id IN ?", userID, ids).Find(&rows).Error
	return rows, err
}

func (r *Repository) UserInstalledSkillBySource(userID, sourceSkillID string) (*model.UserInstalledSkill, error) {
	var row model.UserInstalledSkill
	err := r.db.First(&row, "user_id = ? AND source_skill_id = ?", userID, sourceSkillID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) RecordUserInstalledSkillAgentUse(userID string, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Model(&model.UserInstalledSkill{}).
		Where("user_id = ? AND id IN ?", userID, ids).
		Updates(map[string]any{"agent_use_count": gorm.Expr("COALESCE(agent_use_count, 0) + 1"), "agent_last_used_at": at}).Error
}

func (r *Repository) CreateUserInstalledSkill(row *model.UserInstalledSkill) error {
	return r.db.Create(row).Error
}

func (r *Repository) UpdateUserInstalledSkill(id string, fields map[string]any) error {
	return r.db.Model(&model.UserInstalledSkill{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repository) DeleteUserInstalledSkill(userID, id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var instance model.UserInstalledSkill
		if err := tx.First(&instance, "id = ? AND user_id = ?", id, userID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.UserInstalledSkill{}, "id = ?", instance.ID).Error; err != nil {
			return err
		}
		return deleteUnreferencedSkillContent(tx, instance.ContentID)
	})
}

func (r *Repository) UserSkillFavoriteExists(userID, skillID string) (bool, error) {
	var count int64
	err := r.db.Model(&model.UserSkillFavorite{}).Where("user_id = ? AND skill_id = ?", userID, skillID).Count(&count).Error
	return count > 0, err
}

func (r *Repository) SetUserSkillFavorite(userID, skillID string, liked bool) error {
	if !liked {
		return r.db.Delete(&model.UserSkillFavorite{}, "user_id = ? AND skill_id = ?", userID, skillID).Error
	}
	row := model.UserSkillFavorite{UserID: userID, SkillID: skillID, CreatedAt: time.Now()}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

type MarketSkillFilter struct {
	UserID                string
	ExcludeOwnerID        string
	Search                string
	Tag                   string
	PlatformRootID        string
	PlatformCategoryID    string
	PlatformUncategorized bool
	FavoritesOnly         bool
	Sort                  string
	Limit                 int
	Offset                int
}

type MarketSkillRow struct {
	SkillID           string    `gorm:"column:skill_id"`
	Name              string    `gorm:"column:name"`
	Description       string    `gorm:"column:description"`
	ContentID         string    `gorm:"column:content_id"`
	ContentHash       string    `gorm:"column:content_hash"`
	VersionLabel      string    `gorm:"column:version_label"`
	SourceType        string    `gorm:"column:source_type"`
	SourceURL         string    `gorm:"column:source_url"`
	SourceRef         string    `gorm:"column:source_ref"`
	SourceSubdir      string    `gorm:"column:source_subdir"`
	SourceCommit      string    `gorm:"column:source_commit"`
	ShowcaseMediaJSON string    `gorm:"column:showcase_media_json"`
	ExtraInfo         string    `gorm:"column:extra_info"`
	Tag               string    `gorm:"column:tag"`
	OwnerID           string    `gorm:"column:owner_id"`
	AuthorName        string    `gorm:"column:author_name"`
	ListingStatus     string    `gorm:"column:listing_status"`
	SortWeight        int       `gorm:"column:sort_weight"`
	FileCount         int       `gorm:"column:file_count"`
	TotalBytes        int64     `gorm:"column:total_bytes"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
	AddedCount        int64     `gorm:"column:added_count"`
	LikeCount         int64     `gorm:"column:like_count"`
	InstanceID        string    `gorm:"column:instance_id"`
	IsLike            bool      `gorm:"column:is_like"`
}

const marketSkillSelect = `skills.id AS skill_id, skills.name, skills.description,
	skills.content_id, skills.content_hash, skills.version_label,
	skills.source_type, skills.source_url, skills.source_ref, skills.source_subdir, skills.source_commit,
	skills.showcase_media_json, skills.extra_info, skills.tag, skills.owner_id, skills.author_name,
	skills.created_at, skills.updated_at, skills.file_count, skills.total_bytes,
	l.status AS listing_status, l.sort_weight,
	(SELECT COUNT(*) FROM user_installed_skills c WHERE c.source_skill_id = skills.id) AS added_count,
	(SELECT COUNT(*) FROM user_skill_favorites fc WHERE fc.skill_id = skills.id) AS like_count,
	COALESCE(i.id, '') AS instance_id,
	CASE WHEN f.user_id IS NULL THEN false ELSE true END AS is_like`

func (r *Repository) marketSkillsQuery(filter MarketSkillFilter) *gorm.DB {
	query := r.db.Table("skills").
		Joins("JOIN market_skill_listings l ON l.skill_id = skills.id").
		Joins("LEFT JOIN user_installed_skills i ON i.source_skill_id = skills.id AND i.user_id = ?", filter.UserID).
		Joins("LEFT JOIN user_skill_favorites f ON f.skill_id = skills.id AND f.user_id = ?", filter.UserID).
		Joins("LEFT JOIN users skill_owner ON skill_owner.id = skills.owner_id")
	if filter.FavoritesOnly {
		query = query.Where("f.user_id IS NOT NULL")
	} else {
		query = query.Where("l.status = ?", "published")
	}
	if filter.ExcludeOwnerID != "" {
		query = query.Where("(skills.owner_id IS NULL OR skills.owner_id <> ?)", filter.ExcludeOwnerID)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		query = query.Where("(lower(skills.name) LIKE ? OR lower(skills.description) LIKE ? OR lower(skills.author_name) LIKE ? OR lower(skill_owner.display_name) LIKE ? OR lower(skill_owner.username) LIKE ?)",
			pattern, pattern, pattern, pattern, pattern)
	}
	if filter.Tag != "" {
		query = query.Where("skills.tag = ?", filter.Tag)
	}
	if filter.PlatformRootID != "" {
		query = query.Where(effectiveCurationRoot+" = ?", filter.PlatformRootID)
	}
	if filter.PlatformCategoryID != "" {
		query = query.Where("EXISTS ("+effectiveCurationCategory+" AND c.id = ?)", filter.PlatformCategoryID)
	}
	if filter.PlatformUncategorized {
		query = query.Where("NOT EXISTS (" + effectiveCurationCategory + ")")
	}
	return query
}

func (r *Repository) MarketSkills(filter MarketSkillFilter) ([]MarketSkillRow, int64, error) {
	countQuery := r.marketSkillsQuery(filter)
	var total int64
	if err := countQuery.Distinct("skills.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order := "l.sort_weight DESC, skills.updated_at DESC"
	switch filter.Sort {
	case "new":
		order = "skills.created_at DESC"
	case "popular":
		order = "(SELECT COUNT(*) FROM user_installed_skills metric_installs WHERE metric_installs.source_skill_id = skills.id) DESC, skills.updated_at DESC"
	case "updated":
		order = "skills.updated_at DESC"
	}
	var rows []MarketSkillRow
	err := r.marketSkillsQuery(filter).Select(marketSkillSelect).Order(order).Limit(filter.Limit).Offset(filter.Offset).Scan(&rows).Error
	return rows, total, err
}

func (r *Repository) MarketSkill(userID, skillID string) (*MarketSkillRow, error) {
	var row MarketSkillRow
	err := r.marketSkillsQuery(MarketSkillFilter{UserID: userID}).Select(marketSkillSelect).Where("skills.id = ?", skillID).Limit(1).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.SkillID == "" {
		return nil, nil
	}
	return &row, nil
}

func (r *Repository) SkillSocialState(userID, skillID string) (int64, int64, bool, error) {
	var added, liked int64
	if err := r.db.Model(&model.UserInstalledSkill{}).Where("source_skill_id = ?", skillID).Count(&added).Error; err != nil {
		return 0, 0, false, err
	}
	if err := r.db.Model(&model.UserSkillFavorite{}).Where("skill_id = ?", skillID).Count(&liked).Error; err != nil {
		return 0, 0, false, err
	}
	isLike, err := r.UserSkillFavoriteExists(userID, skillID)
	return added, liked, isLike, err
}

func (r *Repository) UpsertMarketSkillListing(listing *model.MarketSkillListing) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "skill_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "sort_weight", "showcase_media_json", "extra_info", "published_at", "published_by", "updated_at",
		}),
	}).Create(listing).Error
}

func (r *Repository) HideMarketSkillListing(skillID string) error {
	return r.db.Model(&model.MarketSkillListing{}).Where("skill_id = ?", skillID).
		Updates(map[string]any{"status": "hidden", "updated_at": time.Now()}).Error
}

func (r *Repository) UserSkillLibraryCategories(userID string) ([]model.UserSkillLibraryCategory, error) {
	var rows []model.UserSkillLibraryCategory
	err := r.db.Where("user_id = ?", userID).Order("normalized_name ASC, created_at ASC").Find(&rows).Error
	return rows, err
}

func (r *Repository) UserSkillLibraryCategoryCounts(userID string) (map[string]int64, error) {
	type countRow struct {
		CategoryID string `gorm:"column:library_category_id"`
		Count      int64  `gorm:"column:count"`
	}
	var rows []countRow
	err := r.db.Model(&model.UserInstalledSkill{}).
		Select("COALESCE(library_category_id, '') AS library_category_id, COUNT(*) AS count").
		Where("user_id = ?", userID).Group("library_category_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.CategoryID] = row.Count
	}
	return result, nil
}

func (r *Repository) UserSkillLibraryCategory(id string) (*model.UserSkillLibraryCategory, error) {
	var row model.UserSkillLibraryCategory
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repository) CreateUserSkillLibraryCategory(row *model.UserSkillLibraryCategory) (bool, error) {
	var count int64
	if err := r.db.Model(&model.UserSkillLibraryCategory{}).
		Where("user_id = ? AND normalized_name = ?", row.UserID, row.NormalizedName).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := r.db.Create(row).Error; err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) DeleteUserSkillLibraryCategory(userID, id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserInstalledSkill{}).
			Where("user_id = ? AND library_category_id = ?", userID, id).
			Update("library_category_id", "").Error; err != nil {
			return err
		}
		result := tx.Delete(&model.UserSkillLibraryCategory{}, "id = ? AND user_id = ?", id, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *Repository) CreateUserSkillWithContent(instance *model.UserInstalledSkill, content *model.SkillContent, files []model.SkillContentFile) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(content).Error; err != nil {
			return err
		}
		if len(files) > 0 {
			if err := tx.CreateInBatches(&files, 100).Error; err != nil {
				return err
			}
		}
		return tx.Create(instance).Error
	})
}

func (r *Repository) ReplaceUserSkillContent(instanceID string, previousContentID string, fields map[string]any, content *model.SkillContent, files []model.SkillContentFile) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(content).Error; err != nil {
			return err
		}
		if len(files) > 0 {
			if err := tx.CreateInBatches(&files, 100).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.UserInstalledSkill{}).Where("id = ?", instanceID).Updates(fields).Error; err != nil {
			return err
		}
		return deleteUnreferencedSkillContent(tx, previousContentID)
	})
}
