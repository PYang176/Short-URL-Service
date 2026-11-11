package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// PasswordParams 定义 Argon2id 的时间和内存成本
type PasswordParams struct {
	// Memory 是每次哈希使用的 KiB 内存
	Memory uint32
	// Iterations 是 Argon2 迭代次数
	Iterations uint32
	// Parallelism 是并行计算通道数
	Parallelism uint8
	// SaltLength 是随机盐字节数
	SaltLength uint32
	// KeyLength 是最终摘要字节数
	KeyLength uint32
}

// DefaultPasswordParams 是服务端交互登录使用的默认成本参数
var DefaultPasswordParams = PasswordParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// PasswordHasher 使用 Argon2id 生成和校验密码摘要
type PasswordHasher struct {
	params PasswordParams
}

// NewPasswordHasher 创建密码摘要器，Memory 为零时使用安全默认参数
func NewPasswordHasher(params PasswordParams) *PasswordHasher {
	if params.Memory == 0 {
		params = DefaultPasswordParams
	}
	return &PasswordHasher{params: params}
}

// Hash 为明文密码生成带随机盐的 Argon2id 编码摘要
func (h *PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成密码盐失败: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password),
		salt,
		h.params.Iterations,
		h.params.Memory,
		h.params.Parallelism,
		h.params.KeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		h.params.Memory,
		h.params.Iterations,
		h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify 使用摘要内参数重新计算并以常量时间比较密码
func (h *PasswordHasher) Verify(password, encodedHash string) (bool, error) {
	params, salt, expected, err := decodePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// decodePasswordHash 解析 Argon2id 编码并提取参数、盐和摘要
func decodePasswordHash(encodedHash string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return PasswordParams{}, nil, nil, errors.New("密码摘要格式无效")
	}

	var params PasswordParams
	for _, item := range strings.Split(parts[3], ",") {
		keyValue := strings.SplitN(item, "=", 2)
		if len(keyValue) != 2 {
			return PasswordParams{}, nil, nil, errors.New("密码摘要参数无效")
		}
		value, err := strconv.ParseUint(keyValue[1], 10, 32)
		if err != nil {
			return PasswordParams{}, nil, nil, errors.New("密码摘要参数无效")
		}
		switch keyValue[0] {
		case "m":
			params.Memory = uint32(value)
		case "t":
			params.Iterations = uint32(value)
		case "p":
			if value > 255 {
				return PasswordParams{}, nil, nil, errors.New("密码摘要并行参数无效")
			}
			params.Parallelism = uint8(value)
		default:
			return PasswordParams{}, nil, nil, errors.New("密码摘要包含未知参数")
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, errors.New("密码盐无效")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, errors.New("密码摘要值无效")
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))
	return params, salt, key, nil
}
