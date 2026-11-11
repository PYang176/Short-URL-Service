package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	tokenIssuer   = "shorturl"
	tokenAudience = "shorturl-api"
)

// Claims 是 HTTP 中间件使用的 Access Token 业务声明
type Claims struct {
	// WorkspaceID 是当前请求操作的工作空间 ULID
	WorkspaceID string `json:"wid"`
	// Role 是用户在工作空间中的角色
	Role uint8 `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager 负责签发和校验 HMAC Access Token
type TokenManager struct {
	secret []byte
	ttl    time.Duration
}

// NewTokenManager 创建指定密钥和有效期的 Access Token 管理器
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl}
}

// Issue 为指定用户和工作空间签发 Access Token
func (m *TokenManager) Issue(userID, workspaceID string, role uint8, now time.Time) (string, time.Time, error) {
	expires := now.Add(m.ttl)
	claims := Claims{
		WorkspaceID: workspaceID,
		Role:        role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{tokenAudience},
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("签发 Access Token 失败: %w", err)
	}
	return signed, expires, nil
}

// Parse 校验签名、Issuer、Audience 和过期时间并返回业务声明
func (m *TokenManager) Parse(tokenString string, now time.Time) (Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("access Token 签名算法不符合预期")
			}
			return m.secret, nil
		},
		jwt.WithAudience(tokenAudience),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.Subject == "" || claims.WorkspaceID == "" {
		return Claims{}, ErrUnauthorized
	}
	return *claims, nil
}

// GenerateRefreshToken 生成随机不透明 Refresh Token 及其 SHA-256 摘要
func GenerateRefreshToken() (string, []byte, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", nil, fmt.Errorf("生成 Refresh Token 失败: %w", err)
	}
	token := "srt_" + base64.RawURLEncoding.EncodeToString(random)
	hash := sha256.Sum256([]byte(token))
	return token, hash[:], nil
}

// HashRefreshToken 计算用于数据库查找的 Refresh Token 摘要
func HashRefreshToken(token string) []byte {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hash[:]
}
