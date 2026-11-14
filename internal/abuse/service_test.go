package abuse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// TestParseReportedSlug 验证公开举报可以从完整短链或路径提取短码
func TestParseReportedSlug(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "完整地址", raw: "https://short.example.com/r/abcd", want: "abcd"},
		{name: "路径", raw: "/r/abcd", want: "abcd"},
		{name: "裸短码", raw: "abcd", want: "abcd"},
		{name: "空值", raw: "", wantErr: true},
		{name: "多级路径", raw: "https://short.example.com/r/abcd/extra", wantErr: true},
		{name: "非法字符", raw: "/r/ab cd", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseReportedSlug(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseReportedSlug() = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parseReportedSlug() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

// TestNormalizeBlockedHostname 验证域名和 IP 黑名单规则会被规范化
func TestNormalizeBlockedHostname(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "域名", raw: "Bad.Example.COM.", want: "bad.example.com"},
		{name: "IPv4", raw: "192.0.2.1", want: "192.0.2.1"},
		{name: "URL", raw: "https://bad.example", wantErr: true},
		{name: "带路径", raw: "bad.example/path", wantErr: true},
		{name: "空值", raw: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeBlockedHostname(test.raw)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeBlockedHostname() = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("normalizeBlockedHostname() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

// TestServiceCreateReportResolvesLinkOwnership 验证举报使用权威短链归属
func TestServiceCreateReportResolvesLinkOwnership(t *testing.T) {
	repo := &memoryRepository{}
	gateway := &memoryLinkGateway{
		result: link.ResolveResult{
			LinkID:      "link-1",
			WorkspaceID: "workspace-1",
			Slug:        "abcd",
		},
	}
	service := NewService(repo, gateway)
	service.now = func() time.Time {
		return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	}

	result, err := service.CreateReport(
		context.Background(),
		"http://localhost:18088/r/abcd",
		ReasonPhishing,
		"页面冒充登录入口",
	)
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	if result.Status != ReportStatusPending || repo.report == nil {
		t.Fatalf("CreateReport() result = %+v, report = %+v", result, repo.report)
	}
	if repo.report.LinkID != "link-1" || repo.report.WorkspaceID != "workspace-1" {
		t.Fatalf("举报归属 = %+v", repo.report)
	}
}

// TestServiceTakedownInvalidatesCache 验证举报下架成功后会失效短链缓存
func TestServiceTakedownInvalidatesCache(t *testing.T) {
	repo := &memoryRepository{
		resolution: &ReportResolution{
			Report:   Report{ID: "report-1", LinkID: "link-1"},
			DomainID: "domain-1",
			Slug:     "abcd",
		},
	}
	gateway := &memoryLinkGateway{}
	service := NewService(repo, gateway)

	if _, err := service.ResolveReport(
		context.Background(),
		"report-1",
		"admin-1",
		DecisionTakedown,
		"确认钓鱼",
	); err != nil {
		t.Fatalf("ResolveReport() error = %v", err)
	}
	if gateway.invalidatedDomain != "domain-1" || gateway.invalidatedSlug != "abcd" {
		t.Fatalf("缓存失效参数 = %q, %q", gateway.invalidatedDomain, gateway.invalidatedSlug)
	}
}

// TestServiceRejectsInvalidReport 验证非法举报原因和已失效短链被拒绝
func TestServiceRejectsInvalidReport(t *testing.T) {
	repo := &memoryRepository{}
	gateway := &memoryLinkGateway{resolveErr: link.ErrGone}
	service := NewService(repo, gateway)

	if _, err := service.CreateReport(
		context.Background(),
		"http://localhost/r/abcd",
		"unknown",
		"",
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("非法原因 error = %v", err)
	}
	if _, err := service.CreateReport(
		context.Background(),
		"http://localhost/r/abcd",
		ReasonOther,
		"",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("失效链接 error = %v", err)
	}
}

// TestDestinationCheckerExtractsHostname 验证链接策略适配器向仓库传入 Host
func TestDestinationCheckerExtractsHostname(t *testing.T) {
	repo := &memoryRepository{blocked: true}
	checker := NewDestinationChecker(repo)
	blocked, err := checker.IsDestinationBlocked(
		context.Background(),
		"https://Login.Blocked.Example/path?q=1",
	)
	if err != nil || !blocked {
		t.Fatalf("IsDestinationBlocked() blocked=%v error=%v", blocked, err)
	}
	if repo.lastHostname != "login.blocked.example" {
		t.Fatalf("仓库收到 Host = %q", repo.lastHostname)
	}
}

type memoryRepository struct {
	report       *Report
	resolution   *ReportResolution
	blocked      bool
	lastHostname string
}

// CreateReport 保存测试举报
func (r *memoryRepository) CreateReport(_ context.Context, report *Report) error {
	copy := *report
	r.report = &copy
	return nil
}

// ListReports 返回空举报列表
func (r *memoryRepository) ListReports(context.Context, uint8, int) ([]Report, error) {
	return []Report{}, nil
}

// ResolveReport 返回预置举报处理结果
func (r *memoryRepository) ResolveReport(
	context.Context,
	string, string, string, string,
	time.Time,
) (*ReportResolution, error) {
	if r.resolution == nil {
		return nil, ErrNotFound
	}
	return r.resolution, nil
}

// ListBlockedDomains 返回空黑名单
func (r *memoryRepository) ListBlockedDomains(context.Context) ([]BlockedDomain, error) {
	return []BlockedDomain{}, nil
}

// AddBlockedDomain 保存测试黑名单规则
func (r *memoryRepository) AddBlockedDomain(_ context.Context, domain *BlockedDomain) error {
	r.blocked = true
	return nil
}

// RemoveBlockedDomain 忽略测试移除操作
func (r *memoryRepository) RemoveBlockedDomain(
	context.Context,
	string, string,
	time.Time,
) error {
	return nil
}

// IsDestinationBlocked 返回测试阻断结果
func (r *memoryRepository) IsDestinationBlocked(_ context.Context, hostname string) (bool, error) {
	r.lastHostname = hostname
	return r.blocked, nil
}

type memoryLinkGateway struct {
	result            link.ResolveResult
	resolveErr        error
	invalidatedDomain string
	invalidatedSlug   string
}

// Resolve 返回测试短链解析结果
func (g *memoryLinkGateway) Resolve(context.Context, string) (link.ResolveResult, error) {
	return g.result, g.resolveErr
}

// Invalidate 保存测试缓存失效参数
func (g *memoryLinkGateway) Invalidate(_ context.Context, domainID, slug string) {
	g.invalidatedDomain = domainID
	g.invalidatedSlug = slug
}
