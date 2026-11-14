package link

import (
	"context"
	"encoding/json"
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

	resolved, err := service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.DestinationURL != "https://example.com/start" || resolved.RedirectCode != 302 {
		t.Fatalf("Resolve() = %q, %d; want https://example.com/start, 302", resolved.DestinationURL, resolved.RedirectCode)
	}

	lookupCount := repo.lookupCount()
	if _, err := service.Resolve(context.Background(), created.Slug); err != nil {
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
	resolved, err = service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() after update error = %v", err)
	}
	if resolved.DestinationURL != newDestination {
		t.Fatalf("更新后目标 = %q, want %q", resolved.DestinationURL, newDestination)
	}

	if err := service.Delete(context.Background(), "workspace-1", created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := service.Resolve(context.Background(), created.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("删除后 Resolve() error = %v, want ErrNotFound", err)
	}
}

// TestDeleteCacheRetriesTransientRedisFailure 验证 Redis 短暂重启后仍能完成缓存失效。
func TestDeleteCacheRetriesTransientRedisFailure(t *testing.T) {
	server := miniredis.NewMiniRedis()
	if err := server.Start(); err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(server.Close)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("关闭 Redis 失败: %v", err)
		}
	})

	key := linkCacheKey("domain-1", "retry")
	if err := client.Set(t.Context(), key, "cached", time.Minute).Err(); err != nil {
		t.Fatalf("写入测试缓存失败: %v", err)
	}
	server.Close()
	restarted := make(chan error, 1)
	go func() {
		time.Sleep(75 * time.Millisecond)
		restarted <- server.Restart()
	}()

	service := &Service{
		redis:  client,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	service.deleteCache(t.Context(), "domain-1", "retry")
	if err := <-restarted; err != nil {
		t.Fatalf("重启 miniredis 失败: %v", err)
	}
	exists, err := client.Exists(t.Context(), key).Result()
	if err != nil {
		t.Fatalf("检查缓存失败: %v", err)
	}
	if exists != 0 {
		t.Fatal("Redis 恢复后缓存仍然存在")
	}
}

// TestServiceEmitsOutboxEvents 验证创建、修改和删除都会生成对应 Webhook 事件
func TestServiceEmitsOutboxEvents(t *testing.T) {
	service, repo, cleanup := testService(t)
	defer cleanup()

	created, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/events",
		Alias:          "events",
		Title:          "Events",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	newDestination := "https://example.com/events-updated"
	if _, err := service.Update(
		context.Background(),
		"workspace-1",
		created.ID,
		UpdateInput{DestinationURL: &newDestination},
	); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := service.Delete(
		context.Background(),
		"workspace-1",
		created.ID,
	); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	events := repo.outboxEvents()
	if len(events) != 3 {
		t.Fatalf("Outbox 事件数 = %d, want 3", len(events))
	}
	wantTypes := []string{"link.created", "link.updated", "link.deleted"}
	for index, event := range events {
		if event.Type != wantTypes[index] || event.ID == "" {
			t.Fatalf("事件 %d = %+v", index, event)
		}
		var payload struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("解析事件 %d 失败: %v", index, err)
		}
		if payload.ID != event.ID || payload.Type != event.Type {
			t.Fatalf("事件 %d envelope = %+v", index, payload)
		}
	}
}

// TestServiceRejectsBlockedDestination 验证黑名单目标不能创建或修改为可跳转链接
func TestServiceRejectsBlockedDestination(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	service.destinationPolicy = fakeDestinationPolicy{blocked: true}

	_, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://blocked.example/phishing",
		Alias:          "blocked",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
	}

	service.destinationPolicy = fakeDestinationPolicy{}
	created, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://allowed.example/start",
		Alias:          "allowed",
	})
	if err != nil {
		t.Fatalf("Create() allowed error = %v", err)
	}
	service.destinationPolicy = fakeDestinationPolicy{blocked: true}
	destination := "https://blocked.example/after-update"
	_, err = service.Update(
		context.Background(),
		"workspace-1",
		created.ID,
		UpdateInput{DestinationURL: &destination},
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Update() error = %v, want ErrInvalidInput", err)
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

	resolved, err := service.Resolve(context.Background(), created.Slug)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.DestinationURL != "https://example.com/fallback" {
		t.Fatalf("Resolve() = %q, want fallback destination", resolved.DestinationURL)
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
	if _, err := service.Resolve(context.Background(), future.Slug); !errors.Is(err, ErrNotStarted) {
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
	if _, err := service.Resolve(context.Background(), expired.Slug); !errors.Is(err, ErrGone) {
		t.Fatalf("过期链接 Resolve() error = %v, want ErrGone", err)
	}
}

// TestServiceCustomDomain 验证自定义域名创建、返回地址和 Host 跳转
func TestServiceCustomDomain(t *testing.T) {
	service, _, cleanup := testService(t)
	defer cleanup()
	service.domains = fakeDomainAuthorizer{}

	created, err := service.Create(context.Background(), "workspace-1", CreateInput{
		DestinationURL: "https://example.com/custom",
		Alias:          "custom",
		DomainID:       "domain-custom",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.DomainHostname != "go.example.com" {
		t.Fatalf("DomainHostname = %q", created.DomainHostname)
	}
	if shortURL := service.ShortURL(created); shortURL != "http://go.example.com/custom" {
		t.Fatalf("ShortURL() = %q", shortURL)
	}

	resolved, err := service.ResolveByHost(context.Background(), "go.example.com", "custom")
	if err != nil {
		t.Fatalf("ResolveByHost() error = %v", err)
	}
	if resolved.DomainID != "domain-custom" || resolved.DestinationURL != "https://example.com/custom" {
		t.Fatalf("ResolveByHost() = %+v", resolved)
	}
}

// testService 创建使用内存仓库和 miniredis 的短链接服务
func testService(t *testing.T) (*Service, *fakeRepository, func()) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	repo := newFakeRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service, err := NewService(repo, redisClient, logger, "http://localhost:8080", nil)
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
	events      []OutboxEvent
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
func (r *fakeRepository) Create(_ context.Context, link *Link, event *OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := link.DomainID + ":" + link.Slug
	if _, exists := r.links[key]; exists {
		return ErrSlugTaken
	}
	r.links[key] = cloneLink(link)
	r.recordEvent(event)
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
func (r *fakeRepository) Update(_ context.Context, link *Link, event *OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := link.DomainID + ":" + link.Slug
	if _, ok := r.links[key]; !ok {
		return ErrNotFound
	}
	r.links[key] = cloneLink(link)
	r.recordEvent(event)
	return nil
}

// SoftDelete 标记测试链接已删除
func (r *fakeRepository) SoftDelete(
	_ context.Context,
	workspaceID, id string,
	now time.Time,
	event *OutboxEvent,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, link := range r.links {
		if link.WorkspaceID == workspaceID && link.ID == id && link.DeletedAt == nil {
			link.DeletedAt = &now
			link.Version++
			r.recordEvent(event)
			return nil
		}
	}
	return ErrNotFound
}

// recordEvent 保存测试事件副本
func (r *fakeRepository) recordEvent(event *OutboxEvent) {
	if event == nil {
		return
	}
	copy := *event
	copy.Payload = append([]byte(nil), event.Payload...)
	r.events = append(r.events, copy)
}

// outboxEvents 返回测试仓库记录的 Outbox 事件
func (r *fakeRepository) outboxEvents() []OutboxEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]OutboxEvent, len(r.events))
	for index, event := range r.events {
		events[index] = event
		events[index].Payload = append([]byte(nil), event.Payload...)
	}
	return events
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

type fakeDomainAuthorizer struct{}

// AuthorizeLinkDomain 返回固定已验证自定义域名
func (fakeDomainAuthorizer) AuthorizeLinkDomain(context.Context, string, string) (string, error) {
	return "go.example.com", nil
}

type fakeDestinationPolicy struct {
	blocked bool
}

// IsDestinationBlocked 返回测试配置的目标阻断结果
func (f fakeDestinationPolicy) IsDestinationBlocked(
	context.Context,
	string,
) (bool, error) {
	return f.blocked, nil
}

// ResolveVerifiedHost 返回固定自定义域名 ID
func (fakeDomainAuthorizer) ResolveVerifiedHost(context.Context, string) (string, error) {
	return "domain-custom", nil
}
