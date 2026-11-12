package link

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestServiceCreateResolveAndInvalidate 验证创建、缓存命中、修改失效和删除
func TestServiceCreateResolveAndInvalidate(t *testing.T) {
	service, repo, cleanup := testService(t)
	defer cleanup()

	created, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "Example.com/start",
		Alias:          "launch",
		Title:          "Launch",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	destination, code, err := service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if destination != "https://example.com/start" || code != 302 {
		t.Fatalf("Resolve() = %q, %d; want https://example.com/start, 302", destination, code)
	}

	lookupCount := repo.lookupCount()
	if _, _, err := service.Resolve(context.Background(), created.Slug); err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if repo.lookupCount() != lookupCount {
		t.Fatal("缓存命中时仍然回源 MySQL")
	}

	newDestination := "https://example.com/changed"
	if _, err := service.Update(context.Background(), "workspace-1", created.ID, UpdateInput{
		DestinationURL: &newDestination,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	destination, _, err = service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() after update error = %v", err)
	}
	if destination != newDestination {
		t.Fatalf("更新后目标 = %q, want %q", destination, newDestination)
	}

	if err := service.Delete(context.Background(), "workspace-1", created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := service.Resolve(context.Background(), created.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 Resolve() error = %v, want ErrNotFound", err)
	}
}

// TestServiceResolveFallsBackWhenRedisUnavailable 验证 Redis 故障时仍可回源 MySQL
func TestServiceResolveFallsBackWhenRedisUnavailable(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()

	created, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/fallback",
		Alias:          "fallback",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := service.redis.Close(); err != nil {
		t.Fatalf("关闭 Redis 失败: %v", err)
	}

	destination, _, err := service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if destination != "https://example.com/fallback" {
		t.Fatalf("Resolve() = %q, want fallback destination", destination)
	}
}

// TestServiceResolveLinkWindow 验证未开始和已过期链接被拒绝
func TestServiceResolveLinkWindow(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()

	now := service.now()
	startsAt := now.Add(time.Hour)
	future, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/future",
		Alias:          "future",
		StartsAt:       &startsAt,
	})
	if err != nil {
		t.Fatalf("Create() future error = %v", err)
	}
	if _, _, err := service.Resolve(context.Background(), future.Slug); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("未开始链接 Resolve() error = %v, want ErrNotStarted", err)
	}

	expiresAt := now.Add(-time.Minute)
	expired, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/expired",
		Alias:          "expired",
		ExpiresAt:      &expiresAt,
	})
	if err != nil {
		t.Fatalf("Create() expired error = %v", err)
	}
	if _, _, err := service.Resolve(context.Background(), expired.Slug); !errors.Is(err, ErrGone) {
		t.Fatalf("过期链接 Resolve() error = %v, want ErrGone", err)
	}
}

// testService 创建使用内存仓库和 miniredis 的短链接服务
func testService(t *testing.T) (*Service, *fakeRepository, func()) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	repo := newFakeRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service, err := NewService(repo, redisClient, logger, "http://localhost:8080")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	}
	if err := service.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return service, repo, func() {
		_ = redisClient.Close()
	}
}

type fakeRepository struct {
	mu          sync.Mutex
	defaultHost string
	links       map[string]*Link
	lookups     int
}

// newFakeRepository 创建隔离的内存短链接仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{links: make(map[string]*Link)}
}

// EnsureDefaultDomain 返回固定测试默认域名
func (r *fakeRepository) EnsureDefaultDomain(_ context.Context, hostname string, now time.Time) (*Domain, error) {
	r.defaultHost = hostname
	return &Domain{
		ID:        "domain-1",
		Hostname:  hostname,
		Status:    DomainStatusVerified,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Create 保存测试链接并模拟短码唯一约束
func (r *fakeRepository) Create(_ context.Context, link *Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := link.DomainID + ":" + link.Slug
	if _, exists := r.links[key]; exists {
		return ErrSlugTaken
	}
	r.links[key] = cloneLink(link)
	return nil
}

// FindByID 按工作空间和 ID 返回测试链接
func (r *fakeRepository) FindByID(_ context.Context, workspaceID, id string) (*Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, link := range r.links {
		if link.WorkspaceID == workspaceID && link.ID == id && link.DeletedAt == nil {
			return cloneLink(link), nil
		}
	}
	return nil, ErrNotFound
}

// FindByDomainSlug 按域名和短码返回测试链接并记录回源次数
func (r *fakeRepository) FindByDomainSlug(_ context.Context, domainID, slug string) (*Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lookups++
	link, ok := r.links[domainID+":"+slug]
	if !ok || link.DeletedAt != nil {
		return nil, ErrNotFound
	}
	return cloneLink(link), nil
}

// List 返回按创建时间倒序排列的测试链接
func (r *fakeRepository) List(
	_ context.Context,
	workspaceID string,
	limit int,
	beforeCreatedAt *time.Time,
	beforeID string,
) ([]Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Link, 0, len(r.links))
	for _, link := range r.links {
		if link.WorkspaceID != workspaceID || link.DeletedAt != nil {
			continue
		}
		if beforeCreatedAt != nil && !link.CreatedAt.Before(*beforeCreatedAt) && link.ID >= beforeID {
			continue
		}
		result = append(result, *cloneLink(link))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// Update 覆盖保存测试链接
func (r *fakeRepository) Update(_ context.Context, link *Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := link.DomainID + ":" + link.Slug
	if _, ok := r.links[key]; !ok {
		return ErrNotFound
	}
	r.links[key] = cloneLink(link)
	return nil
}

// SoftDelete 标记测试链接已删除
func (r *fakeRepository) SoftDelete(_ context.Context, workspaceID, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, link := range r.links {
		if link.WorkspaceID == workspaceID && link.ID == id && link.DeletedAt == nil {
			link.DeletedAt = &now
			link.Version++
			return nil
		}
	}
	return ErrNotFound
}

// lookupCount 返回测试仓库的跳转回源次数
func (r *fakeRepository) lookupCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lookups
}

// cloneLink 深复制链接中的切片和可空时间字段
func cloneLink(link *Link) *Link {
	copy := *link
	copy.DestinationHash = append([]byte(nil), link.DestinationHash...)
	if link.StartsAt != nil {
		value := *link.StartsAt
		copy.StartsAt = &value
	}
	if link.ExpiresAt != nil {
		value := *link.ExpiresAt
		copy.ExpiresAt = &value
	}
	if link.DeletedAt != nil {
		value := *link.DeletedAt
		copy.DeletedAt = &value
	}
	return &copy
}
