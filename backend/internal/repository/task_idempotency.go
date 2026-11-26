package repository

import (
	"time"

	"yingce/backend/internal/model"
)

func (r *Repository) TaskIdempotencyByUserKey(userID, key string) (*model.TaskIdempotency, error) {
	var record model.TaskIdempotency
	if err := r.db.Where("user_id = ? AND key = ?", userID, key).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *Repository) DeleteTaskIdempotencyBefore(cutoff time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 1000
	}
	var total int64
	for {
		expired := r.db.Model(&model.TaskIdempotency{}).Select("id").Where("expires_at < ?", cutoff).Order("expires_at").Limit(batchSize)
		result := r.db.Where("id IN (?)", expired).Delete(&model.TaskIdempotency{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < int64(batchSize) {
			return total, nil
		}
	}
}
