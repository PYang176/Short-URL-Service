package abuse

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

var blockedHostnamePattern = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`,
)

// LinkGateway 定义举报处理需要的短链解析和缓存失效能力
type LinkGateway interface {
	// Resolve 解析默认域名短码
	Resolve(ctx context.Context, slug string) (link.ResolveResult, error)
	// Invalidate 删除指定域名和短码的跳转缓存
	Invalidate(ctx context.Context, domainID, slug string)
}

// Service 提供举报受理、管理员裁决和域名黑名单能力
type Service struct {
	// repo 是治理数据仓库
	repo Repository
	// links 解析被举报短链并失效缓存
	links LinkGateway
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// DestinationChecker 将完整目标 URL 适配为域名仓库的黑名单检查
type DestinationChecker struct {
	// repo 提供域名阻断查询
	repo Repository
}

// NewService 创建滥用治理服务
func NewService(repo Repository, links LinkGateway) *Service {
	return &Service{
		repo:  repo,
		links: links,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// NewDestinationChecker 创建短链接服务使用的目标安全策略适配器
func NewDestinationChecker(repo Repository) *DestinationChecker {
	return &DestinationChecker{repo: repo}
}

// CreateReport 验证公开举报并保存待处理记录
func (s *Service) CreateReport(
	ctx context.Context,
	shortURL, reason, details string,
) (*CreateReportResult, error) {
	slug, err := parseReportedSlug(shortURL)
	if err != nil {
		return nil, err
	}
	reason = strings.ToLower(strings.TrimSpace(reason))
	if !validReason(reason) {
		return nil, fmt.Errorf("%w: 举报类型无效", ErrInvalidInput)
	}
	details = strings.TrimSpace(details)
	if len([]rune(details)) > 1000 {
		return nil, fmt.Errorf("%w: 补充说明不能超过 1000 个字符", ErrInvalidInput)
	}
	resolved, err := s.links.Resolve(ctx, slug)
	if err != nil {
		if errors.Is(err, link.ErrNotFound) || errors.Is(err, link.ErrGone) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("解析被举报短链失败: %w", err)
	}

	now := s.now()
	report := &Report{
		ID:          ulid.Make().String(),
		LinkID:      resolved.LinkID,
		WorkspaceID: resolved.WorkspaceID,
		Reason:      reason,
		Details:     details,
		Status:      ReportStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateReport(ctx, report); err != nil {
		return nil, err
	}
	return &CreateReportResult{ID: report.ID, Status: report.Status}, nil
}

// ListReports 返回指定状态的管理员举报队列
func (s *Service) ListReports(
	ctx context.Context,
	status uint8,
	limit int,
) ([]Report, error) {
	if status < ReportStatusPending || status > ReportStatusRejected {
		return nil, fmt.Errorf("%w: 举报状态无效", ErrInvalidInput)
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit 必须为 1 至 100", ErrInvalidInput)
	}
	return s.repo.ListReports(ctx, status, limit)
}

// ResolveReport 处理举报，并在下架后使短链缓存立即失效
func (s *Service) ResolveReport(
	ctx context.Context,
	reportID, adminUserID, decision, note string,
) (*ReportResolution, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != DecisionTakedown && decision != DecisionReject {
		return nil, fmt.Errorf("%w: 处理决定必须是 takedown 或 reject", ErrInvalidInput)
	}
	note = strings.TrimSpace(note)
	if len([]rune(note)) > 1000 {
		return nil, fmt.Errorf("%w: 处理说明不能超过 1000 个字符", ErrInvalidInput)
	}
	result, err := s.repo.ResolveReport(
		ctx,
		reportID,
		adminUserID,
		decision,
		note,
		s.now(),
	)
	if err != nil {
		return nil, err
	}
	if result.DomainID != "" && result.Slug != "" {
		s.links.Invalidate(ctx, result.DomainID, result.Slug)
	}
	return result, nil
}

// ListBlockedDomains 返回当前目标域名黑名单
func (s *Service) ListBlockedDomains(ctx context.Context) ([]BlockedDomain, error) {
	return s.repo.ListBlockedDomains(ctx)
}

// AddBlockedDomain 规范化域名并新增黑名单规则
func (s *Service) AddBlockedDomain(
	ctx context.Context,
	adminUserID, hostname, reason string,
) (*BlockedDomain, error) {
	normalized, err := normalizeBlockedHostname(hostname)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return nil, fmt.Errorf("%w: 阻断原因长度必须为 1 至 500 个字符", ErrInvalidInput)
	}
	now := s.now()
	domain := &BlockedDomain{
		ID:              ulid.Make().String(),
		Hostname:        normalized,
		Reason:          reason,
		CreatedByUserID: adminUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.AddBlockedDomain(ctx, domain); err != nil {
		return nil, err
	}
	return domain, nil
}

// RemoveBlockedDomain 停用指定黑名单规则
func (s *Service) RemoveBlockedDomain(
	ctx context.Context,
	adminUserID, id string,
) error {
	return s.repo.RemoveBlockedDomain(ctx, id, adminUserID, s.now())
}

// IsDestinationBlocked 判断已规范化 URL 的目标 Host 是否被阻断
func (s *Service) IsDestinationBlocked(
	ctx context.Context,
	destinationURL string,
) (bool, error) {
	return checkDestinationBlocked(ctx, s.repo, destinationURL)
}

// IsDestinationBlocked 检查完整目标 URL 是否命中域名黑名单
func (c *DestinationChecker) IsDestinationBlocked(
	ctx context.Context,
	destinationURL string,
) (bool, error) {
	return checkDestinationBlocked(ctx, c.repo, destinationURL)
}

// checkDestinationBlocked 提取规范化 Host 并查询有效黑名单
func checkDestinationBlocked(
	ctx context.Context,
	repo Repository,
	destinationURL string,
) (bool, error) {
	parsed, err := url.Parse(destinationURL)
	if err != nil || parsed.Hostname() == "" {
		return false, fmt.Errorf("%w: 目标地址无效", ErrInvalidInput)
	}
	return repo.IsDestinationBlocked(ctx, strings.ToLower(parsed.Hostname()))
}

// parseReportedSlug 从完整短链或路径中提取平台默认域名短码
func parseReportedSlug(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", fmt.Errorf("%w: 短链地址无效", ErrInvalidInput)
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return "", fmt.Errorf("%w: 短链地址无效", ErrInvalidInput)
		}
		raw = parsed.Path
	}
	raw = strings.Trim(raw, "/")
	raw = strings.TrimPrefix(raw, "r/")
	if raw == "" || strings.Contains(raw, "/") || !validReportedSlug(raw) {
		return "", fmt.Errorf("%w: 短码格式无效", ErrInvalidInput)
	}
	return raw, nil
}

// validReportedSlug 校验公开举报提交的短码字符和长度
func validReportedSlug(slug string) bool {
	if len(slug) < 4 || len(slug) > 32 {
		return false
	}
	for _, character := range slug {
		valid := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '-'
		if !valid {
			return false
		}
	}
	return true
}

// validReason 判断举报类型是否在允许集合中
func validReason(reason string) bool {
	switch reason {
	case ReasonPhishing, ReasonMalware, ReasonSpam, ReasonImpersonation, ReasonOther:
		return true
	default:
		return false
	}
}

// normalizeBlockedHostname 规范化管理员提交的域名或 IP 规则
func normalizeBlockedHostname(raw string) (string, error) {
	raw = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if raw == "" || len(raw) > 253 {
		return "", fmt.Errorf("%w: 域名长度必须为 1 至 253 个字符", ErrInvalidInput)
	}
	if ip := net.ParseIP(raw); ip != nil {
		return ip.String(), nil
	}
	if strings.ContainsAny(raw, "/:@?#") || !blockedHostnamePattern.MatchString(raw) {
		return "", fmt.Errorf("%w: 域名格式无效", ErrInvalidInput)
	}
	return raw, nil
}
