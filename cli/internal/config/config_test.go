package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	creds := Credentials{
		Token:     "yc_test",
		TokenID:   "tok_1",
		UserID:    "user_1",
		Scopes:    []string{"read"},
		ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
	if err := SaveCredentials(dir, creds); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unexpected permissions %o", info.Mode().Perm())
	}
	_, loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != creds.Token || loaded.UserID != creds.UserID {
		t.Fatalf("unexpected credentials %+v", loaded)
	}
	if err := ClearCredentials(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("credentials file still exists")
	}
}
