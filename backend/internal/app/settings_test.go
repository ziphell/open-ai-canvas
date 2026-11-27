package app

import (
	"testing"

	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMissingOSSSettingsReturnNormalizedDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.SystemSetting{}, &model.UserOSSSetting{}, &model.StorageLocation{}); err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())
	admin := &model.User{ID: "admin", Role: model.UserRoleAdmin}

	platformSetting, err := svc.AdminOSSSetting(admin)
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultOSSSetting(t, platformSetting)

	userSetting, err := svc.UserOSSSetting(&model.User{ID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultOSSSetting(t, userSetting)
}

func assertDefaultOSSSetting(t *testing.T, value *PublicOSSSetting) {
	t.Helper()
	if value.Provider != aliyunOSSProvider || value.PathPrefix != defaultOSSPathPrefix || value.S3Preset != "custom" {
		t.Fatalf("default OSS setting = %+v", value)
	}
}

func TestUserModelDefaultsPersistPerAccount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserModelPreference{}); err != nil {
		t.Fatal(err)
	}
	svc := New(repository.New(db), t.TempDir())
	user := &model.User{ID: "model-default-user"}

	defaults, err := svc.UserModelDefaults(user)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.ImageModel != "" || defaults.VideoModel != "" || defaults.TextModel != "" || defaults.AudioModel != "" {
		t.Fatalf("missing defaults = %+v", defaults)
	}

	saved, err := svc.UpdateUserModelDefaults(user, UpdateUserModelDefaultsRequest{
		ImageModel: " channel-a::image-1 ",
		VideoModel: "channel-a::video-1",
		TextModel:  "channel-a::text-1",
		AudioModel: "channel-a::audio-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ImageModel != "channel-a::image-1" || saved.VideoModel != "channel-a::video-1" || saved.TextModel != "channel-a::text-1" || saved.AudioModel != "channel-a::audio-1" {
		t.Fatalf("saved defaults = %+v", saved)
	}

	if _, err := svc.UpdateUserModelDefaults(user, UpdateUserModelDefaultsRequest{ImageModel: "image-2"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := svc.UserModelDefaults(user)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ImageModel != "image-2" || reloaded.VideoModel != "" || reloaded.TextModel != "" || reloaded.AudioModel != "" {
		t.Fatalf("overwritten defaults = %+v", reloaded)
	}
}
