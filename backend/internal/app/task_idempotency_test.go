package app

import (
	"errors"
	"testing"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func idempotencyTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+newID()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskIdempotency{}); err != nil {
		t.Fatal(err)
	}
	return &Service{repo: repository.New(db)}, db
}

func TestPrepareTaskIdempotencyReplaysAndConflicts(t *testing.T) {
	svc, db := idempotencyTestService(t)
	key := "cli-key-12345678"
	input := map[string]any{"mode": "image", "prompt": "draw"}
	prompt := "draw"
	if err := db.Create(&model.Task{
		ID: "task-1", UserID: "user-1", Type: "canvas_image", Status: model.TaskStatusQueued, Prompt: prompt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	digest := taskIdempotencyDigest("user-1", "canvas_image", "image", "", "", prompt, input)
	if err := db.Create(&model.TaskIdempotency{
		ID: "idem-1", UserID: "user-1", Key: key, RequestDigest: digest, TaskID: "task-1",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}

	record, replay, err := svc.prepareTaskIdempotency("user-1", "canvas_image", CreateTaskRequest{
		IdempotencyKey: key, Operation: "image", Prompt: prompt,
	}, prompt, input)
	if err != nil {
		t.Fatal(err)
	}
	if record != nil || replay == nil || replay.ID != "task-1" || !replay.Replayed {
		t.Fatalf("expected replay of task-1, got record=%v replay=%+v", record, replay)
	}

	_, _, err = svc.prepareTaskIdempotency("user-1", "canvas_image", CreateTaskRequest{
		IdempotencyKey: key, Operation: "image", Prompt: "different",
	}, "different", input)
	var appErr *kernel.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || string(appErr.Reason) != "idempotency_conflict" {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	record, replay, err = svc.prepareTaskIdempotency("user-1", "canvas_image", CreateTaskRequest{
		IdempotencyKey: "cli-key-87654321", Operation: "image", Prompt: prompt,
	}, prompt, input)
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || replay != nil || record.TaskID != "" {
		t.Fatalf("expected new idempotency record, got record=%+v replay=%+v", record, replay)
	}
}

func TestValidateTaskIdempotencyKey(t *testing.T) {
	if err := validateTaskIdempotencyKey("short"); err == nil {
		t.Fatal("short key should fail")
	}
	if err := validateTaskIdempotencyKey("bad key with spaces"); err == nil {
		t.Fatal("invalid characters should fail")
	}
	if err := validateTaskIdempotencyKey("cli-key-12345678"); err != nil {
		t.Fatal(err)
	}
}
