package skills

import (
	"errors"
	"testing"
	"time"

	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSkillLibraryCategoriesVisibilityAssignmentAndDelete(t *testing.T) {
	svc, db := newSkillLibraryCategoryTestService(t)
	user := createSkillTestUser(t, db, "user-1", model.UserRoleUser, model.UserStatusActive)
	otherUser := createSkillTestUser(t, db, "user-2", model.UserRoleUser, model.UserStatusActive)
	member := createSkillTestUser(t, db, "member", model.UserRoleUser, model.UserStatusActive)
	admin := createSkillTestUser(t, db, "admin-1", model.UserRoleAdmin, model.UserStatusActive)

	personal, err := svc.CreateSkillLibraryCategory(user, SkillLibraryCategoryMutationRequest{Name: "  Writer\tRoom  "})
	if err != nil {
		t.Fatal(err)
	}
	if personal.Name != "Writer Room" || personal.Scope != "personal" {
		t.Fatalf("personal category = %#v", personal)
	}
	if _, err := svc.CreateSkillLibraryCategory(user, SkillLibraryCategoryMutationRequest{Name: "writer room"}); appErrorStatus(err) != 400 {
		t.Fatalf("duplicate category error = %v, want 400", err)
	}
	second, err := svc.CreateSkillLibraryCategory(user, SkillLibraryCategoryMutationRequest{Name: "Characters"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSkillLibraryCategory(otherUser, SkillLibraryCategoryMutationRequest{Name: "writer room"}); err != nil {
		t.Fatalf("same personal name for another user should be allowed: %v", err)
	}
	for _, actor := range []*model.User{member, admin} {
		if _, err := svc.CreateSkillLibraryCategory(actor, SkillLibraryCategoryMutationRequest{Name: "Shared", Scope: "platform"}); appErrorStatus(err) != 400 {
			t.Fatalf("platform category must be rejected for %s: %v", actor.ID, err)
		}
	}

	now := time.Now()
	source := func(value string) *string { return &value }
	instances := []model.UserInstalledSkill{
		{ID: "owned", UserID: user.ID, Name: "Owned", Description: "d", ContentID: "content-owned", ContentHash: "h", VersionNumber: 1, LibraryCategoryID: personal.ID, InstalledAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: "joined", UserID: user.ID, SourceSkillID: source("catalog-joined"), Name: "Joined", Description: "d", ContentID: "content-joined", ContentHash: "h", VersionNumber: 1, LibraryCategoryID: second.ID, InstalledAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: "joined-uncategorized", UserID: user.ID, SourceSkillID: source("catalog-uncategorized"), Name: "Joined uncategorized", Description: "d", ContentID: "content-uncategorized", ContentHash: "h", VersionNumber: 1, InstalledAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: "other-joined", UserID: otherUser.ID, SourceSkillID: source("catalog-other"), Name: "Other", Description: "d", ContentID: "content-other", ContentHash: "h", VersionNumber: 1, InstalledAt: now, CreatedAt: now, UpdatedAt: now},
	}
	for index := range instances {
		if err := db.Create(&instances[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if _, err := svc.SetSkillLibraryCategory(user.ID, "owned", personal.ID); err != nil {
		t.Fatalf("categorize owned instance: %v", err)
	}
	if _, err := svc.SetSkillLibraryCategory(user.ID, "not-joined", personal.ID); appErrorStatus(err) != 404 {
		t.Fatalf("categorize missing instance error = %v, want 404", err)
	}
	if _, err := svc.SetSkillLibraryCategory(otherUser.ID, "joined", personal.ID); appErrorStatus(err) != 404 {
		t.Fatalf("categorize another user's instance error = %v, want 404", err)
	}
	if _, err := svc.SetSkillLibraryCategory(user.ID, "joined", ""); err != nil {
		t.Fatalf("clear joined category: %v", err)
	}
	if _, err := svc.SetSkillLibraryCategory(user.ID, "joined", second.ID); err != nil {
		t.Fatalf("assign personal category: %v", err)
	}

	userCategories, err := svc.SkillLibraryCategories(user.ID, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if userCategories.TotalCount != 3 || userCategories.UncategorizedCount != 1 || categoryCount(userCategories.Categories, personal.ID) != 1 || categoryCount(userCategories.Categories, second.ID) != 1 {
		t.Fatalf("mine category counts = %#v", userCategories)
	}
	otherCategories, err := svc.SkillLibraryCategories(otherUser.ID, "mine")
	if err != nil || len(otherCategories.Categories) != 1 || hasLibraryCategory(otherCategories.Categories, personal.ID) {
		t.Fatalf("other user's categories = %#v %v", otherCategories, err)
	}

	personalSkills, err := svc.Skills(user.ID, SkillListRequest{Scope: "mine", PageSize: 80, LibraryCategoryID: personal.ID})
	if err != nil || personalSkills.TotalCount != 1 || personalSkills.Skills[0].SkillID != "owned" {
		t.Fatalf("personal category filter = %#v %v", personalSkills, err)
	}
	if _, err := svc.Skills(user.ID, SkillListRequest{Scope: "created", LibraryCategoryID: personal.ID, LibraryUncategorized: true}); appErrorStatus(err) != 400 {
		t.Fatalf("conflicting library filters error = %v, want 400", err)
	}
	secondSkills, err := svc.Skills(user.ID, SkillListRequest{Scope: "mine", PageSize: 80, LibraryCategoryID: second.ID})
	if err != nil || secondSkills.TotalCount != 1 || secondSkills.Skills[0].SkillID != "joined" {
		t.Fatalf("second category filter = %#v %v", secondSkills, err)
	}
	uncategorized, err := svc.Skills(user.ID, SkillListRequest{Scope: "mine", PageSize: 80, LibraryUncategorized: true})
	if err != nil || uncategorized.TotalCount != 1 || uncategorized.Skills[0].SkillID != "joined-uncategorized" {
		t.Fatalf("uncategorized filter = %#v %v", uncategorized, err)
	}

	if err := svc.DeleteSkillLibraryCategory(user, personal.ID); err != nil {
		t.Fatal(err)
	}
	var owned model.UserInstalledSkill
	if err := db.First(&owned, "id = ?", "owned").Error; err != nil || owned.LibraryCategoryID != "" {
		t.Fatalf("deleted personal category reference = %#v %v", owned, err)
	}
	var instanceCount int64
	if err := db.Model(&model.UserInstalledSkill{}).Count(&instanceCount).Error; err != nil || instanceCount != 4 {
		t.Fatalf("instance count after category deletion = %d %v", instanceCount, err)
	}
	if err := svc.DeleteSkillLibraryCategory(member, second.ID); appErrorStatus(err) != 404 {
		t.Fatalf("delete another user's category error = %v, want 404", err)
	}
	if err := svc.DeleteSkillLibraryCategory(user, second.ID); err != nil {
		t.Fatal(err)
	}
	var joined model.UserInstalledSkill
	if err := db.First(&joined, "id = ?", "joined").Error; err != nil || joined.LibraryCategoryID != "" {
		t.Fatalf("deleted category reference = %#v %v", joined, err)
	}
}

func newSkillLibraryCategoryTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+kernel.NewID()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Skill{}, &model.User{}, &model.UserIdentity{}, &model.SkillContent{}, &model.SkillContentFile{}, &model.MarketSkillListing{}, &model.UserInstalledSkill{}, &model.UserSkillFavorite{}, &model.UserSkillLibraryCategory{}); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(db), t.TempDir(), nil), db
}

func appErrorStatus(err error) int {
	var appErr *kernel.AppError
	if errors.As(err, &appErr) {
		return appErr.Status
	}
	return 0
}

func hasLibraryCategory(categories []SkillLibraryCategory, id string) bool {
	for _, category := range categories {
		if category.ID == id {
			return true
		}
	}
	return false
}

func categoryCount(categories []SkillLibraryCategory, id string) int64 {
	for _, category := range categories {
		if category.ID == id {
			return category.Count
		}
	}
	return 0
}
