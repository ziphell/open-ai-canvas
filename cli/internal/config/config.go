package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// SkillTarget 记录 setup 写入过的 SKILL.md 目录，供 setup --update 刷新。
type SkillTarget struct {
	Agent string `json:"agent,omitempty"`
	Path  string `json:"path"`
}

// Config is the non-secret CLI configuration.
type Config struct {
	BaseURL          string        `json:"baseUrl,omitempty"`
	DefaultProjectID string        `json:"defaultProjectId,omitempty"`
	SkillTargets     []SkillTarget `json:"skillTargets,omitempty"`
}

// Credentials is the local token record. It never contains a password or cookie.
type Credentials struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"tokenId,omitempty"`
	UserID    string    `json:"userId,omitempty"`
	Scopes    []string  `json:"scopes,omitempty"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func Dir(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if value := os.Getenv("YINGCE_CONFIG_DIR"); value != "" {
		return value, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "yingce"), nil
}

func Load(dir string) (Config, Credentials, error) {
	var cfg Config
	var creds Credentials
	if err := readJSON(filepath.Join(dir, "config.json"), &cfg); err != nil {
		return cfg, creds, err
	}
	if err := readJSON(filepath.Join(dir, "credentials.json"), &creds); err != nil {
		return cfg, creds, err
	}
	return cfg, creds, nil
}

func SaveConfig(dir string, cfg Config) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "config.json"), append(payload, '\n'))
}

func SaveCredentials(dir string, creds Credentials) error {
	if creds.Token == "" {
		return errors.New("token 不能为空")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "credentials.json"), append(payload, '\n'))
}

func writeAtomic(path string, payload []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".yingce-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ClearCredentials(dir string) error {
	err := os.Remove(filepath.Join(dir, "credentials.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func readJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, target)
}
