package webhook

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const (
	// secretPrefix 是 Webhook 签名 Secret 的展示前缀
	secretPrefix = "whsec_"
)

// SecretCipher 使用 AES-GCM 加解密 Webhook 签名 Secret
type SecretCipher struct {
	// aead 是带认证的加密器
	aead cipher.AEAD
}

// NewSecretCipher 从应用密钥派生专用的 Webhook 加密密钥
func NewSecretCipher(applicationSecret string) (*SecretCipher, error) {
	if len(strings.TrimSpace(applicationSecret)) < 32 {
		return nil, fmt.Errorf("webhook 加密源密钥至少需要 32 字节")
	}
	derived := sha256.Sum256([]byte("shorturl:webhook:v1:" + applicationSecret))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, fmt.Errorf("初始化 Webhook 加密器失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化 Webhook AES-GCM 失败: %w", err)
	}
	return &SecretCipher{aead: aead}, nil
}

// GenerateSecret 生成新的随机会话密钥
func GenerateSecret() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("生成 Webhook Secret 失败: %w", err)
	}
	return secretPrefix + base64.RawURLEncoding.EncodeToString(random), nil
}

// Encrypt 使用随机 Nonce 加密 Secret，并将 Nonce 放在密文前
func (c *SecretCipher) Encrypt(secret string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 Webhook Nonce 失败: %w", err)
	}
	return c.aead.Seal(nonce, nonce, []byte(secret), nil), nil
}

// Decrypt 验证并解密 Secret
func (c *SecretCipher) Decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) <= c.aead.NonceSize() {
		return "", fmt.Errorf("webhook Secret 密文无效")
	}
	nonce := ciphertext[:c.aead.NonceSize()]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext[c.aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("解密 Webhook Secret 失败: %w", err)
	}
	return string(plaintext), nil
}
