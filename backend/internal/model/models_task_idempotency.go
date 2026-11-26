package model

import "time"

// TaskIdempotency binds a client-supplied idempotency key to one task.
// It is written in the same transaction as the task and its credit reservation.
type TaskIdempotency struct {
	ID            string    `json:"id" gorm:"primaryKey;size:36"`
	UserID        string    `json:"userId" gorm:"size:36;not null;uniqueIndex:idx_task_idempotency_user_key"`
	Key           string    `json:"key" gorm:"size:128;not null;uniqueIndex:idx_task_idempotency_user_key"`
	RequestDigest string    `json:"requestDigest" gorm:"size:64;not null"`
	TaskID        string    `json:"taskId" gorm:"size:36;not null;index"`
	CreatedAt     time.Time `json:"createdAt" gorm:"index"`
	ExpiresAt     time.Time `json:"expiresAt" gorm:"index"`
}

func (TaskIdempotency) TableName() string { return "task_idempotency" }
