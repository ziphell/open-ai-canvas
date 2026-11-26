package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

const taskIdempotencyTTL = 7 * 24 * time.Hour

// prepareTaskIdempotency returns an existing task for a replayed key, or a new
// record to be inserted in the same transaction as the task.
func (s *Service) prepareTaskIdempotency(userID, taskType string, req CreateTaskRequest, prompt string, input map[string]any) (*model.TaskIdempotency, *model.Task, error) {
	key := strings.TrimSpace(req.IdempotencyKey)
	if key == "" {
		return nil, nil, nil
	}
	if err := validateTaskIdempotencyKey(key); err != nil {
		return nil, nil, err
	}
	digest := taskIdempotencyDigest(userID, taskType, req.Operation, req.LogicalModelID, req.Model, prompt, input)
	existing, err := s.replayTaskIdempotency(userID, key, digest)
	if err == nil {
		return nil, existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	now := time.Now()
	return &model.TaskIdempotency{
		ID:            newID(),
		UserID:        userID,
		Key:           key,
		RequestDigest: digest,
		CreatedAt:     now,
		ExpiresAt:     now.Add(taskIdempotencyTTL),
	}, nil, nil
}

func (s *Service) replayTaskIdempotency(userID, key, digest string) (*model.Task, error) {
	record, err := s.repo.TaskIdempotencyByUserKey(userID, key)
	if err != nil {
		return nil, err
	}
	if record.RequestDigest != digest {
		return nil, idempotencyConflictError()
	}
	task, err := s.repo.Task(record.TaskID)
	if err != nil {
		return nil, err
	}
	if task.UserID != userID {
		return nil, idempotencyConflictError()
	}
	task.Replayed = true
	return task, nil
}

func taskIdempotencyDigest(userID, taskType, operation, logicalModelID, modelCode, prompt string, input map[string]any) string {
	payload := map[string]any{
		"userId":         userID,
		"type":           strings.TrimSpace(taskType),
		"operation":      strings.TrimSpace(operation),
		"logicalModelId": strings.TrimSpace(logicalModelID),
		"model":          strings.TrimSpace(modelCode),
		"prompt":         prompt,
		"input":          input,
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validateTaskIdempotencyKey(key string) error {
	if len(key) < 8 || len(key) > 128 {
		return BadAuthRequest("Idempotency-Key 长度必须为 8–128 个字符")
	}
	for _, char := range key {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			continue
		}
		return BadAuthRequest("Idempotency-Key 只能包含字母、数字、- 和 _")
	}
	return nil
}

func idempotencyConflictError() error {
	return &kernel.AppError{
		Status:  409,
		Code:    409,
		Reason:  kernel.ErrorReason("idempotency_conflict"),
		Message: "幂等键已用于不同请求",
	}
}
