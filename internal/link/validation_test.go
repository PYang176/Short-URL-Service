package link

import (
	"errors"
	"strings"
	"testing"

	"github.com/PYang176/Short-URL-Service/pkg/base62"
)

// TestNormalizeDestination 验证目标地址补全、规范化和危险输入拒绝
func TestNormalizeDestination(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "补全 HTTPS", input: "Example.com/path?q=1", want: "https://example.com/path?q=1"},
		{name: "保留 HTTP", input: "http://example.com", want: "http://example.com"},
		{name: "拒绝 FTP", input: "ftp://example.com/file", wantErr: true},
		{name: "拒绝用户信息", input: "https://user:pass@example.com", wantErr: true},
		{name: "拒绝空地址", input: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := normalizeDestination(test.input)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("normalizeDestination() error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDestination() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("normalizeDestination() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestGenerateShortCode 验证随机短码长度和字符集
func TestGenerateShortCode(t *testing.T) {
	code, err := generateShortCode(randomShortCodeLength)
	if err != nil {
		t.Fatalf("generateShortCode() error = %v", err)
	}
	if len(code) != randomShortCodeLength {
		t.Fatalf("短码长度 = %d, want %d", len(code), randomShortCodeLength)
	}
	for _, character := range code {
		if !strings.ContainsRune(base62.Charset, character) {
			t.Fatalf("短码包含非法字符 %q", character)
		}
	}
}

// TestValidateAlias 验证自定义短码边界
func TestValidateAlias(t *testing.T) {
	valid := []string{"abcd", "Promo_2026", "spring-sale"}
	for _, alias := range valid {
		if err := validateAlias(alias); err != nil {
			t.Errorf("validateAlias(%q) error = %v", alias, err)
		}
	}
	invalid := []string{"a", "bad slug", "中文短码", strings.Repeat("a", 33)}
	for _, alias := range invalid {
		if err := validateAlias(alias); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("validateAlias(%q) error = %v, want ErrInvalidInput", alias, err)
		}
	}
}
