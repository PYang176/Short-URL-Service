package domain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	// verifyModeMock 在本地直接模拟 DNS 验证成功
	verifyModeMock = "mock"
	// verifyModeDNS 使用真实 DNS TXT 和 CNAME
	verifyModeDNS = "dns"
)

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Service 提供域名添加、验证和 Host 解析
type Service struct {
	// repo 是域名权威数据仓库
	repo Repository
	// resolver 查询真实 DNS
	resolver Resolver
	// verifyMode 是 mock 或 dns
	verifyMode string
	// cname 是用户域名应指向的平台接入域名
	cname string
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建自定义域名服务
func NewService(repo Repository, resolver Resolver, verifyMode, cname string) *Service {
	return &Service{
		repo:       repo,
		resolver:   resolver,
		verifyMode: verifyMode,
		cname:      strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), "."),
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Add 添加待验证的自定义域名
func (s *Service) Add(ctx context.Context, workspaceID, rawHostname string) (*Domain, error) {
	hostname, err := normalizeHostname(rawHostname)
	if err != nil {
		return nil, err
	}
	token, err := verificationToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	result := &Domain{
		ID:                ulid.Make().String(),
		WorkspaceID:       workspaceID,
		Hostname:          hostname,
		Status:            StatusPending,
		VerificationToken: token,
		ExpectedCNAME:     s.cname,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.Create(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// List 返回工作空间域名
func (s *Service) List(ctx context.Context, workspaceID string) ([]Domain, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Verify 验证域名 DNS 配置或使用 mock 模式直接成功
func (s *Service) Verify(ctx context.Context, workspaceID, domainID string) (*Domain, error) {
	result, err := s.repo.FindByID(ctx, workspaceID, domainID)
	if err != nil {
		return nil, err
	}
	if result.Status == StatusVerified {
		return result, nil
	}

	now := s.now()
	result.VerificationAttempts++
	result.UpdatedAt = now

	verified := s.verifyMode == verifyModeMock
	if !verified {
		verified, err = s.verifyDNS(ctx, result)
		if err != nil {
			result.Status = StatusFailed
			nextVerify := now.Add(time.Minute)
			result.NextVerifyAt = &nextVerify
			_ = s.repo.UpdateVerification(ctx, result)
			return nil, fmt.Errorf("%w: %v", ErrVerificationFailed, err)
		}
	}
	if !verified {
		result.Status = StatusFailed
		nextVerify := now.Add(time.Minute)
		result.NextVerifyAt = &nextVerify
		_ = s.repo.UpdateVerification(ctx, result)
		return nil, ErrVerificationFailed
	}

	result.Status = StatusVerified
	result.LastVerifiedAt = &now
	result.NextVerifyAt = nil
	if err := s.repo.UpdateVerification(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// Delete 删除没有链接在用的自定义域名
func (s *Service) Delete(ctx context.Context, workspaceID, domainID string) error {
	count, err := s.repo.CountLinks(ctx, domainID)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrDomainInUse
	}
	return s.repo.SoftDelete(ctx, workspaceID, domainID, s.now())
}

// ResolveHost 返回已验证域名
func (s *Service) ResolveHost(ctx context.Context, hostname string) (*Domain, error) {
	normalized, err := normalizeHostname(hostname)
	if err != nil {
		return nil, ErrNotFound
	}
	return s.repo.FindVerifiedByHostname(ctx, normalized)
}

// ResolveVerifiedHost 返回已验证 Host 的 ULID 域名标识
func (s *Service) ResolveVerifiedHost(ctx context.Context, hostname string) (string, error) {
	result, err := s.ResolveHost(ctx, hostname)
	if err != nil {
		return "", err
	}
	return result.ID, nil
}

// AuthorizeLinkDomain 校验域名归属和验证状态，并返回规范化 Host
func (s *Service) AuthorizeLinkDomain(
	ctx context.Context,
	workspaceID, domainID string,
) (string, error) {
	result, err := s.repo.FindByID(ctx, workspaceID, domainID)
	if err != nil {
		return "", err
	}
	if result.Status != StatusVerified {
		return "", ErrNotVerified
	}
	return result.Hostname, nil
}

// verifyDNS 校验 TXT 所有权和 CNAME 接入记录
func (s *Service) verifyDNS(ctx context.Context, result *Domain) (bool, error) {
	txtRecords, err := s.resolver.LookupTXT(ctx, "_shorturl."+result.Hostname)
	if err != nil {
		return false, fmt.Errorf("查询 TXT 记录失败: %w", err)
	}
	tokenMatched := false
	for _, value := range txtRecords {
		if strings.TrimSpace(value) == result.VerificationToken {
			tokenMatched = true
			break
		}
	}
	if !tokenMatched {
		return false, fmt.Errorf("TXT 记录不匹配")
	}

	cname, err := s.resolver.LookupCNAME(ctx, result.Hostname)
	if err != nil {
		return false, fmt.Errorf("查询 CNAME 记录失败: %w", err)
	}
	cname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")
	if cname != strings.TrimSuffix(result.ExpectedCNAME, ".") {
		return false, fmt.Errorf("CNAME 应指向 %s", result.ExpectedCNAME)
	}
	return true, nil
}

// normalizeHostname 将用户输入转换为小写、无端口和无路径的域名
func normalizeHostname(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", fmt.Errorf("%w: 域名不能为空", ErrInvalidDomain)
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("%w: 域名格式错误", ErrInvalidDomain)
		}
		raw = parsed.Host
	} else if strings.Contains(raw, "/") {
		parsed, err := url.Parse("https://" + raw)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("%w: 域名格式错误", ErrInvalidDomain)
		}
		raw = parsed.Host
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	raw = strings.TrimSuffix(raw, ".")
	if len(raw) > 253 || net.ParseIP(raw) != nil {
		return "", fmt.Errorf("%w: 域名格式错误", ErrInvalidDomain)
	}

	labels := strings.Split(raw, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("%w: 请输入完整域名", ErrInvalidDomain)
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return "", fmt.Errorf("%w: 域名标签格式错误", ErrInvalidDomain)
		}
	}
	return raw, nil
}

// verificationToken 生成 DNS TXT 所有权验证值
func verificationToken() (string, error) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("生成域名验证值失败: %w", err)
	}
	return "shortuniq-" + base64.RawURLEncoding.EncodeToString(random), nil
}
