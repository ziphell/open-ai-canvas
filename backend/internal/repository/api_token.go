package repository

import (
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

func (r *Repository) CreateAPIToken(token *model.APIToken) error {
	return r.db.Create(token).Error
}

func (r *Repository) APITokenByHash(hash string) (*model.APIToken, error) {
	var token model.APIToken
	if err := r.db.First(&token, "token_hash = ?", hash).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *Repository) ListAPITokensForUser(userID string) ([]model.APIToken, error) {
	var tokens []model.APIToken
	err := r.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

func (r *Repository) TouchAPIToken(id string, now time.Time) error {
	return r.db.Model(&model.APIToken{}).
		Where("id = ? AND (last_used_at IS NULL OR last_used_at < ?)", id, now.Add(-time.Minute)).
		Update("last_used_at", now).Error
}

func (r *Repository) RevokeAPITokenForUser(userID, id string, now time.Time) (bool, error) {
	result := r.db.Model(&model.APIToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", now)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) CreateCLIAuthCode(code *model.CLIAuthCode) error {
	return r.db.Create(code).Error
}

func (r *Repository) CLIAuthCodeByHash(hash string) (*model.CLIAuthCode, error) {
	var code model.CLIAuthCode
	if err := r.db.First(&code, "code_hash = ?", hash).Error; err != nil {
		return nil, err
	}
	return &code, nil
}

// ConsumeCLIAuthCodeAndCreateToken burns the one-time code and inserts the token
// in one transaction. It returns false when the code was already used or expired.
func (r *Repository) ConsumeCLIAuthCodeAndCreateToken(codeID string, now time.Time, token *model.APIToken) (bool, error) {
	consumed := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.CLIAuthCode{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", codeID, now).
			Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		consumed = true
		return tx.Create(token).Error
	})
	return consumed, err
}
