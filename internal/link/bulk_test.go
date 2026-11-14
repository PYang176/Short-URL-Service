package link

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestBulkCreatePartialSuccess 验证合法行成功、非法和冲突行仅自身失败
func TestBulkCreatePartialSuccess(t *testing.T) {
	service, repo, cleanup := testService(t)
	defer cleanup()

	csvBody := `destination_url,alias,title,redirect_code
https://example.com/alpha,bulka,Alpha,301
file:///etc/passwd,bulkbad,Bad,302
https://example.com/duplicate,bulka,Duplicate,302
`
	result, err := service.BulkCreate(
		context.Background(),
		"workspace-1",
		strings.NewReader(csvBody),
	)
	if err != nil {
		t.Fatalf("BulkCreate() error = %v", err)
	}
	if result.Total != 3 || result.Succeeded != 1 || result.Failed != 2 {
		t.Fatalf("BulkCreate() = %+v", result)
	}
	if result.Items[0].Status != "created" || result.Items[0].Link.RedirectCode != 301 {
		t.Fatalf("第一行结果 = %+v", result.Items[0])
	}
	if result.Items[1].ErrorCode != "INVALID_INPUT" {
		t.Fatalf("第二行结果 = %+v", result.Items[1])
	}
	if result.Items[2].ErrorCode != "SLUG_TAKEN" {
		t.Fatalf("第三行结果 = %+v", result.Items[2])
	}
	if len(repo.links) != 1 {
		t.Fatalf("成功写入链接数 = %d, want 1", len(repo.links))
	}
}

// TestBulkCreateRejectsInvalidFileStructure 验证文件级错误不会写入任何行
func TestBulkCreateRejectsInvalidFileStructure(t *testing.T) {
	service, repo, cleanup := testService(t)
	defer cleanup()

	tests := []string{
		"",
		"unknown_header\nvalue\n",
		"alias,title\nbulk,Missing destination\n",
		"destination_url\n",
	}
	for _, body := range tests {
		if _, err := service.BulkCreate(
			context.Background(),
			"workspace-1",
			strings.NewReader(body),
		); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("BulkCreate(%q) error = %v", body, err)
		}
	}
	if len(repo.links) != 0 {
		t.Fatalf("文件级错误仍写入 %d 条链接", len(repo.links))
	}
}

// TestBulkCreateRejectsMoreThan500Rows 验证单文件行数上限
func TestBulkCreateRejectsMoreThan500Rows(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	var body strings.Builder
	body.WriteString("destination_url\n")
	for index := 0; index < maxBulkRows+1; index++ {
		body.WriteString("https://example.com/")
		body.WriteString("bulk")
		body.WriteString(string(rune('a' + index%26)))
		body.WriteByte('\n')
	}
	if _, err := service.BulkCreate(
		context.Background(),
		"workspace-1",
		strings.NewReader(body.String()),
	); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("超过行数上限 error = %v", err)
	}
}
