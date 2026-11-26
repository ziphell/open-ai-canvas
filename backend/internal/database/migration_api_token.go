package database

import (
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func migrateAPITokens(tx *gorm.DB) error {
	return tx.AutoMigrate(&model.APIToken{}, &model.CLIAuthCode{})
}
