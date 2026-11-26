package repository

import (
	"testing"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTaskWithIdempotencyIsAtomic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:task-idem-"+time.Now().Format("150405.000000")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Task{}, &model.TaskIdempotency{}, &model.BillingOrder{}, &model.CreditAccount{}, &model.CreditLedgerEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CreditAccount{UserID: "user-1", AvailableMicrocredits: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	task := &model.Task{ID: "task-1", UserID: "user-1", Type: "canvas_image", Status: model.TaskStatusQueued, Prompt: "draw"}
	order := &model.BillingOrder{
		ID: "order-1", UserID: "user-1", TaskID: task.ID, Model: "test", Capability: "image",
		BillingMode: "fixed_request", UnitPriceMicrocredits: 100, Quantity: 1,
		AmountMicrocredits: 100, ReservedAmountMicrocredits: 100, Status: model.BillingStatusReserved,
	}
	idempotency := &model.TaskIdempotency{
		ID: "idem-1", UserID: "user-1", Key: "key-12345678", RequestDigest: "digest-1", TaskID: task.ID,
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.CreateTaskWithCreditReservationIdempotent(task, order, idempotency, 10); err != nil {
		t.Fatal(err)
	}
	var account model.CreditAccount
	if err := db.First(&account, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 900 || account.ReservedMicrocredits != 100 {
		t.Fatalf("unexpected account %+v", account)
	}
	var taskCount, idempotencyCount int64
	db.Model(&model.Task{}).Count(&taskCount)
	db.Model(&model.TaskIdempotency{}).Count(&idempotencyCount)
	if taskCount != 1 || idempotencyCount != 1 {
		t.Fatalf("tasks=%d idempotency=%d", taskCount, idempotencyCount)
	}

	duplicateTask := &model.Task{ID: "task-2", UserID: "user-1", Type: "canvas_image", Status: model.TaskStatusQueued, Prompt: "draw"}
	duplicateOrder := &model.BillingOrder{
		ID: "order-2", UserID: "user-1", TaskID: duplicateTask.ID, Model: "test", Capability: "image",
		BillingMode: "fixed_request", UnitPriceMicrocredits: 100, Quantity: 1,
		AmountMicrocredits: 100, ReservedAmountMicrocredits: 100, Status: model.BillingStatusReserved,
	}
	duplicateKey := &model.TaskIdempotency{
		ID: "idem-2", UserID: "user-1", Key: "key-12345678", RequestDigest: "digest-1", TaskID: duplicateTask.ID,
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := repo.CreateTaskWithCreditReservationIdempotent(duplicateTask, duplicateOrder, duplicateKey, 10); err == nil {
		t.Fatal("duplicate key should fail")
	}
	if err := db.First(&account, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 900 || account.ReservedMicrocredits != 100 {
		t.Fatalf("duplicate reservation changed account: %+v", account)
	}
	db.Model(&model.Task{}).Count(&taskCount)
	if taskCount != 1 {
		t.Fatalf("duplicate created a task: %d", taskCount)
	}
}
