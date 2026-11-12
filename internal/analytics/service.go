package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// Service 提供有权限校验的链接点击分析
type Service struct {
	// repo 是点击事件分析仓库
	repo Repository
	// links 用于校验链接归属
	links link.Repository
	// now 提供可替换的 UTC 时间
	now func() time.Time
}

// NewService 创建点击分析服务
func NewService(repo Repository, links link.Repository) *Service {
	return &Service{
		repo:  repo,
		links: links,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// Overview 返回单个链接最近 days 天的分析结果
func (s *Service) Overview(ctx context.Context, workspaceID, linkID string, days int) (*Overview, error) {
	if days < 1 || days > 90 {
		return nil, fmt.Errorf("%w: days 必须为 1 至 90", link.ErrInvalidInput)
	}
	if _, err := s.links.FindByID(ctx, workspaceID, linkID); err != nil {
		return nil, err
	}

	to := s.now()
	from := to.AddDate(0, 0, -days)
	summary, err := s.repo.Summary(ctx, workspaceID, linkID, from, to)
	if err != nil {
		return nil, err
	}
	trend, err := s.repo.Trend(ctx, workspaceID, linkID, from, to, days > 7)
	if err != nil {
		return nil, err
	}

	countries, err := s.repo.Breakdown(ctx, workspaceID, linkID, from, to, "country")
	if err != nil {
		return nil, err
	}
	devices, err := s.repo.Breakdown(ctx, workspaceID, linkID, from, to, "device")
	if err != nil {
		return nil, err
	}
	operatingSystems, err := s.repo.Breakdown(ctx, workspaceID, linkID, from, to, "os")
	if err != nil {
		return nil, err
	}
	browsers, err := s.repo.Breakdown(ctx, workspaceID, linkID, from, to, "browser")
	if err != nil {
		return nil, err
	}
	referrers, err := s.repo.Breakdown(ctx, workspaceID, linkID, from, to, "referrer")
	if err != nil {
		return nil, err
	}

	return &Overview{
		Days:             days,
		Summary:          summary,
		Trend:            trend,
		Countries:        countries,
		Devices:          devices,
		OperatingSystems: operatingSystems,
		Browsers:         browsers,
		Referrers:        referrers,
	}, nil
}
