package link

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PYang176/Short-URL-Service/pkg/base62"
	driver "github.com/go-sql-driver/mysql"
)

const (
	// randomShortCodeLength 是自动生成短码的默认长度
	randomShortCodeLength = 8
	// maxDestinationURLLength 是目标 URL 允许的最大字节数
	maxDestinationURLLength = 2048
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,32}$`)

// normalizeDestination 校验并规范化短链目标 URL
func normalizeDestination(raw string) (string, [32]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxDestinationURLLength {
		return "", [32]byte{}, fmt.Errorf("%w: 目标地址长度必须为 1 至 2048 字节", ErrInvalidInput)
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", [32]byte{}, fmt.Errorf("%w: 目标地址格式错误", ErrInvalidInput)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", [32]byte{}, fmt.Errorf("%w: 只支持 http 和 https", ErrInvalidInput)
	}
	if parsed.Host == "" {
		return "", [32]byte{}, fmt.Errorf("%w: 目标地址缺少主机名", ErrInvalidInput)
	}
	if parsed.User != nil {
		return "", [32]byte{}, fmt.Errorf("%w: 目标地址不能包含用户信息", ErrInvalidInput)
	}

	parsed.Host = strings.ToLower(parsed.Host)
	normalized := parsed.String()
	if len(normalized) > maxDestinationURLLength {
		return "", [32]byte{}, fmt.Errorf("%w: 目标地址长度必须为 1 至 2048 字节", ErrInvalidInput)
	}
	return normalized, sha256.Sum256([]byte(normalized)), nil
}

// validateAlias 校验用户自定义短码
func validateAlias(alias string) error {
	if !aliasPattern.MatchString(alias) {
		return fmt.Errorf("%w: 短码必须为 4 至 32 位字母、数字、下划线或连字符", ErrInvalidInput)
	}
	return nil
}

// validateWindow 校验链接生效与过期时间的相对关系
func validateWindow(startsAt, expiresAt *time.Time) error {
	if startsAt != nil && expiresAt != nil && !startsAt.Before(*expiresAt) {
		return fmt.Errorf("%w: 生效时间必须早于过期时间", ErrInvalidInput)
	}
	return nil
}

// normalizeUTCTime 将可选时间转换为 UTC
func normalizeUTCTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

// generateShortCode 使用加密安全随机数生成指定长度的 Base62 短码
func generateShortCode(length int) (string, error) {
	result := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("生成短码随机数失败: %w", err)
	}

	const charsetLength = byte(len(base62.Charset))
	for index, value := range random {
		// 拒绝大于可均匀映射范围的值，避免取模偏差。

		limit := byte(256 - (256 % int(charsetLength)))
		for value >= limit {
			if _, err := rand.Read(random[index : index+1]); err != nil {
				return "", fmt.Errorf("生成短码随机数失败: %w", err)
			}
			value = random[index]
		}
		result[index] = base62.Charset[value%charsetLength]
	}
	return string(result), nil
}

// isDuplicateKey 判断错误是否表示数据库唯一键冲突
func isDuplicateKey(err error) bool {
	var mysqlError *driver.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1062
}
