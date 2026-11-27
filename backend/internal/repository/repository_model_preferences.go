// 用户默认模型偏好。

package repository

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm/clause"
)

func (r *Repository) UserModelPreference(userID string) (*model.UserModelPreference, error) {
	var preference model.UserModelPreference
	if err := r.db.First(&preference, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &preference, nil
}

func (r *Repository) SaveUserModelPreference(preference *model.UserModelPreference) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"image_model", "video_model", "text_model", "audio_model", "updated_at"}),
	}).Create(preference).Error
}
