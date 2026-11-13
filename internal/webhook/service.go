package webhook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Service 管理 Webhook 端点生命周期和签名密钥
type Service struct {
	// repo 是 Webhook 配置和投递记录仓库
	repo Repository
	// cipher 使用应用密钥加密签名 Secret
	cipher *SecretCipher
	// allowPrivateTargets 控制本地演示是否允许私网 HTTP 地址
	allowPrivateTargets bool
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建 Webhook 管理服务
func NewService(
	repo Repository,
	cipher *SecretCipher,
	allowPrivateTargets bool,
) *Service {
	return &Service{
		repo:                repo,
		cipher:              cipher,
		allowPrivateTargets: allowPrivateTargets,
		now:                 func() time.Time { return time.Now().UTC() },
	}
}

// Create 创建端点并返回仅展示一次的签名 Secret
func (s *Service) Create(
	ctx context.Context,
	workspaceID, name, targetURL string,
) (*EndpointCreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return nil, fmt.Errorf("%w: 名称长度必须为 1 至 64 个字符", ErrInvalidInput)
	}
	normalizedURL, err := ValidateEndpointURL(ctx, targetURL, s.allowPrivateTargets)
	if err != nil {
		return nil, err
	}
	secret, err := GenerateSecret()
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.cipher.Encrypt(secret)
	if err != nil {
		return nil, err
	}

	now := s.now()
	endpoint := Endpoint{
		ID:               ulid.Make().String(),
		WorkspaceID:      workspaceID,
		Name:             name,
		URL:              normalizedURL,
		SecretCiphertext: ciphertext,
		Status:           EndpointStatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.repo.CreateEndpoint(ctx, &endpoint); err != nil {
		return nil, err
	}
	return &EndpointCreateResult{Endpoint: endpoint, Secret: secret}, nil
}

// List 返回当前工作空间未删除的端点
func (s *Service) List(ctx context.Context, workspaceID string) ([]Endpoint, error) {
	return s.repo.ListEndpoints(ctx, workspaceID)
}

// Disable 停用端点并停止后续投递
func (s *Service) Disable(ctx context.Context, workspaceID, id string) error {
	return s.repo.DisableEndpoint(ctx, workspaceID, id, s.now())
}

// ListDeliveries 返回端点最近投递记录
func (s *Service) ListDeliveries(
	ctx context.Context,
	workspaceID, endpointID string,
	limit int,
) ([]Delivery, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit 必须为 1 至 100", ErrInvalidInput)
	}
	return s.repo.ListDeliveries(ctx, workspaceID, endpointID, limit)
}
