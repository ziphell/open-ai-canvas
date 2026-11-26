package model

import "time"

// APIToken 是 CLI 或自动化使用的长期访问令牌。明文只在签发时返回一次，
// 数据库只保存 SHA-256 哈希；Scope 和额度在签发时冻结。
type APIToken struct {
	ID          string     `json:"id" gorm:"primaryKey;size:36"`
	UserID      string     `json:"userId" gorm:"size:36;not null;index"`
	Name        string     `json:"name" gorm:"size:64;not null"`
	TokenPrefix string     `json:"tokenPrefix" gorm:"size:16;not null"`
	TokenHash   string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	ScopesJSON  string     `json:"scopesJson" gorm:"type:text;not null"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt" gorm:"not null;index"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	RevokedAt   *time.Time `json:"revokedAt" gorm:"index"`
}

func (APIToken) TableName() string { return "api_tokens" }

// CLIAuthCode 是一次性浏览器授权码。CodeHash 是授权码的 SHA-256；
// PKCE challenge 与 redirect URI 在兑换时必须精确匹配。
type CLIAuthCode struct {
	ID                  string     `json:"id" gorm:"primaryKey;size:36"`
	CodeHash            string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	UserID              string     `json:"userId" gorm:"size:36;not null;index"`
	ClientName          string     `json:"clientName" gorm:"size:64"`
	RedirectURI         string     `json:"redirectUri" gorm:"size:255;not null"`
	CodeChallenge       string     `json:"-" gorm:"size:64;not null"`
	CodeChallengeMethod string     `json:"-" gorm:"size:8;not null"`
	ScopesJSON          string     `json:"scopesJson" gorm:"type:text;not null"`
	IPHash              string     `json:"-" gorm:"size:64"`
	UserAgentHash       string     `json:"-" gorm:"size:64"`
	ExpiresAt           time.Time  `json:"expiresAt" gorm:"not null;index"`
	ConsumedAt          *time.Time `json:"consumedAt"`
	CreatedAt           time.Time  `json:"createdAt"`
}

func (CLIAuthCode) TableName() string { return "cli_auth_codes" }
