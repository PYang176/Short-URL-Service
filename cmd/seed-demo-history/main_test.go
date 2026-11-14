package main

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

// TestEventTimestampStaysInPast 验证演示事件不会落在当前时间之后。
func TestEventTimestampStaysInPast(t *testing.T) {
	now := time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		dayIndex    int
		linkIndex   int
		sampleIndex int
	}{
		{name: "首日事件", dayIndex: 0, linkIndex: 0, sampleIndex: 0},
		{name: "中间日事件", dayIndex: 3, linkIndex: 2, sampleIndex: 5},
		{name: "当日事件", dayIndex: 6, linkIndex: 3, sampleIndex: 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			timestamp := eventTimestamp(
				now,
				7,
				test.dayIndex,
				test.linkIndex,
				test.sampleIndex,
			)
			if timestamp.After(now) {
				t.Fatalf("eventTimestamp() = %v, 晚于当前时间 %v", timestamp, now)
			}
		})
	}
}

// TestDeterministicULID 验证相同时间和熵生成相同且可解析的 ULID。
func TestDeterministicULID(t *testing.T) {
	timestamp := time.Date(2026, 11, 15, 9, 30, 0, 0, time.UTC)
	entropy := digest("seed", "link", 3, 2, 1)
	first, err := deterministicULID(timestamp, entropy)
	if err != nil {
		t.Fatalf("deterministicULID() error = %v", err)
	}
	second, err := deterministicULID(timestamp, entropy)
	if err != nil {
		t.Fatalf("deterministicULID() second error = %v", err)
	}
	if first != second {
		t.Fatalf("deterministicULID() = %q, second = %q", first, second)
	}
	if _, err := ulid.Parse(first); err != nil {
		t.Fatalf("ULID 不可解析: %v", err)
	}
}
