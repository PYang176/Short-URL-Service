package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PYang176/Short-URL-Service/internal/link"
)

// TestOpenExportValidatesAndMapsFilters 验证导出筛选参数和真人机器人映射
func TestOpenExportValidatesAndMapsFilters(t *testing.T) {
	repo := &fakeAnalyticsRepository{}
	service := NewService(repo, nil)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	reader, err := service.OpenExport(context.Background(), "workspace-1", ExportInput{
		Days:      7,
		Visitor:   "human",
		Device:    "desktop",
		Country:   "cn",
		Browser:   "Chrome",
		UTMSource: "newsletter",
	})
	if err != nil {
		t.Fatalf("OpenExport() error = %v", err)
	}
	if reader == nil {
		t.Fatal("OpenExport() reader = nil")
	}
	filter := repo.lastExportFilter
	if filter.WorkspaceID != "workspace-1" ||
		filter.From != now.AddDate(0, 0, -7) ||
		filter.To != now ||
		filter.IsBot == nil ||
		*filter.IsBot ||
		filter.Device != "desktop" ||
		filter.Country != "CN" ||
		filter.UTMSource != "newsletter" {
		t.Fatalf("导出过滤条件 = %+v", filter)
	}
}

// TestOpenExportRejectsInvalidFilters 验证导出接口拒绝越界和不支持的筛选值
func TestOpenExportRejectsInvalidFilters(t *testing.T) {
	repo := &fakeAnalyticsRepository{}
	service := NewService(repo, nil)
	tests := []ExportInput{
		{Days: 0},
		{Days: 91},
		{Days: 7, Visitor: "unknown"},
		{Days: 7, Device: "watch"},
		{Days: 7, Country: "CHN"},
		{Days: 7, Browser: string(make([]byte, 33))},
	}
	for index, input := range tests {
		if _, err := service.OpenExport(
			context.Background(),
			"workspace-1",
			input,
		); !errors.Is(err, link.ErrInvalidInput) {
			t.Fatalf("测试 %d OpenExport() error = %v", index, err)
		}
	}
}
