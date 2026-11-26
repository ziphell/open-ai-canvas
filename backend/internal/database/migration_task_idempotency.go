package database

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func migrateTaskIdempotency(tx *gorm.DB) error {
	return tx.AutoMigrate(&model.TaskIdempotency{})
}
