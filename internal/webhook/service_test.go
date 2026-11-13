package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSecretCipherRoundTrip 验证 Webhook Secret 可使用同一应用密钥往返加解密
func TestSecretCipherRoundTrip(t *testing.T) {
	first, err := NewSecretCipher(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("NewSecretCipher() error = %v", err)
	}
	second, err := NewSecretCipher(strings.Repeat("b", 32))
	if err != nil {
		t.Fatalf("NewSecretCipher(second) error = %v", err)
	}
	ciphertext, err := first.Encrypt("whsec_round_trip")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	plaintext, err := first.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if plaintext != "whsec_round_trip" {
		t.Fatalf("Decrypt() = %q", plaintext)
	}
	if _, err := second.Decrypt(ciphertext); err == nil {
		t.Fatal("使用不同应用密钥仍能解密 Secret")
	}
}

// TestHTTPClientBlocksPrivateTargetInPublicMode 验证连接阶段仍会阻断私网目标
func TestHTTPClientBlocksPrivateTargetInPublicMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// 连接阶段应被阻断，因此这个处理器只作为不应到达的目标。
	}))
	defer server.Close()

	client := NewHTTPClient(false, time.Second)
	if _, err := client.Get(server.URL); !errors.Is(err, ErrUnsafeTarget) {
		t.Fatalf("HTTPClient.Get() error = %v, want ErrUnsafeTarget", err)
	}
}

// TestValidateEndpointURL 验证本地与公网模式的目标地址安全边界
func TestValidateEndpointURL(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		allowPrivate bool
		wantErr      error
	}{
		{
			name:         "local http",
			raw:          "http://127.0.0.1:9010/hook",
			allowPrivate: true,
		},
		{
			name:         "local loopback blocked",
			raw:          "https://127.0.0.1/hook",
			allowPrivate: false,
			wantErr:      ErrUnsafeTarget,
		},
		{
			name:         "private blocked",
			raw:          "https://192.168.1.10/hook",
			allowPrivate: false,
			wantErr:      ErrUnsafeTarget,
		},
		{
			name:         "HTTP blocked in public mode",
			raw:          "http://example.com/hook",
			allowPrivate: false,
			wantErr:      ErrUnsafeTarget,
		},
		{
			name:         "userinfo blocked",
			raw:          "https://user:pass@example.com/hook",
			allowPrivate: true,
			wantErr:      ErrInvalidInput,
		},
		{
			name:         "unsupported scheme",
			raw:          "file:///etc/passwd",
			allowPrivate: true,
			wantErr:      ErrInvalidInput,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateEndpointURL(context.Background(), test.raw, test.allowPrivate)
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateEndpointURL() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateEndpointURL() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// TestServiceCreateOnlyReturnsSecretOnce 验证 Service 只把明文 Secret 放入创建结果
func TestServiceCreateOnlyReturnsSecretOnce(t *testing.T) {
	repo := &memoryRepository{}
	cipher, err := NewSecretCipher(strings.Repeat("k", 32))
	if err != nil {
		t.Fatalf("NewSecretCipher() error = %v", err)
	}
	service := NewService(repo, cipher, true)
	service.now = func() time.Time {
		return time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	}

	result, err := service.Create(
		context.Background(),
		"workspace-1",
		"本地调试",
		"http://127.0.0.1:9010/hook",
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(result.Secret, secretPrefix) {
		t.Fatalf("Secret = %q, want prefix %q", result.Secret, secretPrefix)
	}
	if string(result.Endpoint.SecretCiphertext) == result.Secret {
		t.Fatal("数据库模型保存了明文 Secret")
	}

	endpoints, err := service.List(context.Background(), "workspace-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("List() length = %d, want 1", len(endpoints))
	}
	if string(endpoints[0].SecretCiphertext) == "" {
		t.Fatal("端点没有保存 Secret 密文")
	}
}

type memoryRepository struct {
	endpoints []Endpoint
}

// CreateEndpoint 保存测试端点
func (r *memoryRepository) CreateEndpoint(_ context.Context, endpoint *Endpoint) error {
	r.endpoints = append(r.endpoints, *endpoint)
	return nil
}

// ListEndpoints 返回测试端点
func (r *memoryRepository) ListEndpoints(context.Context, string) ([]Endpoint, error) {
	return append([]Endpoint(nil), r.endpoints...), nil
}

// DisableEndpoint 标记测试端点停用
func (r *memoryRepository) DisableEndpoint(
	context.Context,
	string, string,
	time.Time,
) error {
	return nil
}

// ListDeliveries 返回空测试投递记录
func (r *memoryRepository) ListDeliveries(
	context.Context,
	string, string,
	int,
) ([]Delivery, error) {
	return []Delivery{}, nil
}

// ClaimDeliveries 返回空测试任务
func (r *memoryRepository) ClaimDeliveries(
	context.Context,
	int,
	time.Time, time.Time,
) ([]DeliveryJob, error) {
	return []DeliveryJob{}, nil
}

// MarkSuccess 忽略测试投递成功状态
func (r *memoryRepository) MarkSuccess(
	context.Context,
	*DeliveryJob,
	int,
	time.Time,
) error {
	return nil
}

// MarkFailure 忽略测试投递失败状态
func (r *memoryRepository) MarkFailure(
	context.Context,
	*DeliveryJob,
	*int,
	string,
	time.Time,
	int,
) error {
	return nil
}
