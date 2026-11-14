package analytics

import "testing"

// TestSafeCSVCell 验证用户字段不会被表格软件当作公式执行
func TestSafeCSVCell(t *testing.T) {
	tests := map[string]string{
		"normal":      "normal",
		"=1+1":        "'=1+1",
		"+SUM(A1:A2)": "'+SUM(A1:A2)",
		"-1":          "'-1",
		"@cmd":        "'@cmd",
		"":            "",
	}
	for input, want := range tests {
		if got := safeCSVCell(input); got != want {
			t.Fatalf("safeCSVCell(%q) = %q, want %q", input, got, want)
		}
	}
}
