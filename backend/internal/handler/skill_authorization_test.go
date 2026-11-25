package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"yingce/backend/internal/auth"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

func skillAuthorizationFixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "authorization.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(
		&model.User{}, &model.UserIdentity{}, &model.AuthSession{},
		&model.Skill{}, &model.SkillContent{}, &model.SkillContentFile{},
		&model.MarketSkillListing{}, &model.UserInstalledSkill{}, &model.UserSkillFavorite{},
		&model.UserSkillLibraryCategory{}, &model.SkillCurationSetting{}, &model.SkillCurationRoot{}, &model.SkillCurationCategory{},
		&model.SkillCurationAssignment{}, &model.SkillCurationRootAssignment{}, &model.AdminAuditEvent{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SkillCurationSetting{ID: 1}).Error; err != nil {
		t.Fatal(err)
	}
	for _, actor := range []model.User{
		{ID: "skill-admin", Username: "skill-admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive},
		{ID: "skill-member", Username: "skill-member", Role: model.UserRoleUser, Status: model.UserStatusActive},
	} {
		if err := db.Create(&actor).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.AuthSession{ID: actor.ID, UserID: actor.ID, TokenHash: auth.HashToken("skill-access-token"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(repository.New(db), dir)
	t.Cleanup(func() { _ = svc.Close() })
	router := gin.New()
	api := router.Group("/api")
	RegisterSkillRoutes(api, svc)
	RegisterAdminSkillRoutes(api, svc)
	return router, db
}

func skillAuthorizationRequest(router *gin.Engine, method, target, actor string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Content-Type", "application/json")
	if actor != "" {
		req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: actor + ".skill-access-token"})
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}

func TestUserSkillRoutesAllowMembersAndAdminRoutesRejectThem(t *testing.T) {
	router, _ := skillAuthorizationFixture(t)
	for _, test := range []struct {
		method string
		target string
		actor  string
		status int
	}{
		{http.MethodGet, "/api/skills?scope=public", "skill-member", http.StatusOK},
		{http.MethodGet, "/api/skills?scope=mine", "skill-member", http.StatusOK},
		{http.MethodGet, "/api/skills/library-categories", "skill-member", http.StatusOK},
		{http.MethodGet, "/api/skills/curation", "skill-member", http.StatusOK},
		{http.MethodGet, "/api/skills/presets", "skill-member", http.StatusOK},
		{http.MethodGet, "/api/admin/skills", "skill-member", http.StatusForbidden},
		{http.MethodGet, "/api/admin/skill-curation", "skill-member", http.StatusForbidden},
		{http.MethodGet, "/api/admin/skills", "skill-admin", http.StatusOK},
	} {
		t.Run(test.method+" "+test.target+"/"+test.actor, func(t *testing.T) {
			out := skillAuthorizationRequest(router, test.method, test.target, test.actor)
			if out.Code != test.status {
				t.Fatalf("status=%d, want %d; body=%s", out.Code, test.status, out.Body.String())
			}
		})
	}
}

func TestUnauthenticatedUserSkillRoutesAreRejected(t *testing.T) {
	router, _ := skillAuthorizationFixture(t)
	for _, target := range []string{"/api/skills?scope=public", "/api/skills/library-categories", "/api/skills/curation", "/api/skills/added"} {
		out := skillAuthorizationRequest(router, http.MethodGet, target, "")
		if out.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d, want 401; body=%s", target, out.Code, out.Body.String())
		}
	}
}

func TestDemotedAdminLosesAdminRoutesButKeepsUserRoutes(t *testing.T) {
	router, db := skillAuthorizationFixture(t)
	if err := db.Model(&model.User{}).Where("id = ?", "skill-admin").Update("role", model.UserRoleUser).Error; err != nil {
		t.Fatal(err)
	}
	if out := skillAuthorizationRequest(router, http.MethodGet, "/api/admin/skills", "skill-admin"); out.Code != http.StatusForbidden {
		t.Fatalf("admin route after demotion = %d %s", out.Code, out.Body.String())
	}
	if out := skillAuthorizationRequest(router, http.MethodGet, "/api/skills?scope=public", "skill-admin"); out.Code != http.StatusOK {
		t.Fatalf("user route after demotion = %d %s", out.Code, out.Body.String())
	}
}
