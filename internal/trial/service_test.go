package trial

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestServiceCreateAndResume 验证游客会话创建、Token 哈希保存和恢复
func TestServiceCreateAndResume(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, 24*time.Hour, 3)
	service.now = func() time.Time { return now }

	session, token, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.HasPrefix(token, "trial_") {
		t.Fatalf("token = %q, want trial_ prefix", token)
	}
	if string(session.TokenHash) == token {
		t.Fatal("TokenHash must not equal raw token")
	}
	if repo.createdName != "游客试用" || !strings.HasPrefix(repo.createdSlug, "trial_") {
		t.Fatalf("created workspace = %q/%q", repo.createdName, repo.createdSlug)
	}

	resumed, err := service.Resume(context.Background(), token)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if resumed.ID != session.ID || resumed.WorkspaceID != session.WorkspaceID {
		t.Fatalf("Resume() = %+v, want %+v", resumed, session)
	}
	if _, err := service.Resume(context.Background(), "trial_missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Resume() missing error = %v", err)
	}
}

// TestServiceRejectsExpiredSession 验证过期 Cookie 不能恢复游客会话
func TestServiceRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, time.Hour, 3)
	service.now = func() time.Time { return now }
	_, token, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	service.now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := service.Resume(context.Background(), token); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("Resume() error = %v, want ErrSessionExpired", err)
	}
}

// TestServiceEnforcesLinkQuota 验证游客链接配额能够原子占用和归还
func TestServiceEnforcesLinkQuota(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, time.Hour, 2)
	service.now = func() time.Time { return now }
	session, _, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for index := 0; index < 2; index++ {
		if err := service.ReserveLink(context.Background(), session.ID); err != nil {
			t.Fatalf("ReserveLink() attempt %d error = %v", index+1, err)
		}
	}
	if err := service.ReserveLink(context.Background(), session.ID); !errors.Is(err, ErrLinkLimitReached) {
		t.Fatalf("ReserveLink() over limit error = %v", err)
	}
	if err := service.ReleaseLink(context.Background(), session.ID); err != nil {
		t.Fatalf("ReleaseLink() error = %v", err)
	}
	if err := service.ReserveLink(context.Background(), session.ID); err != nil {
		t.Fatalf("ReserveLink() after release error = %v", err)
	}
}

// TestServiceLinkExpiryOptions 验证有效期白名单并与游客会话上限取较小值
func TestServiceLinkExpiryOptions(t *testing.T) {
	now := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	service := NewService(newFakeRepository(), 24*time.Hour, 3)
	service.now = func() time.Time { return now }
	session := &Session{ExpiresAt: now.Add(2 * time.Hour)}

	expiresAt, err := service.LinkExpiresAt(session, ExpiryMinutes15)
	if err != nil || !expiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("15 分钟有效期 = %v, %v", expiresAt, err)
	}
	expiresAt, err = service.LinkExpiresAt(session, ExpiryMinutes720)
	if err != nil || !expiresAt.Equal(session.ExpiresAt) {
		t.Fatalf("12 小时有效期应受会话上限约束: %v, %v", expiresAt, err)
	}
	if _, err := service.LinkExpiresAt(session, 45); !errors.Is(err, ErrInvalidExpiry) {
		t.Fatalf("非法有效期 error = %v", err)
	}
}

// TestServiceClaim 验证游客 Token 能认领到登录工作空间并只消费一次
func TestServiceClaim(t *testing.T) {
	now := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	repo := newFakeRepository()
	service := NewService(repo, time.Hour, 3)
	service.now = func() time.Time { return now }
	_, token, err := service.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	result, err := service.Claim(context.Background(), token, "workspace-target")
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if result.Links != 1 || result.Clicks != 2 || repo.claimedTarget != "workspace-target" {
		t.Fatalf("Claim() = %+v, target = %q", result, repo.claimedTarget)
	}
	if _, err := service.Claim(context.Background(), token, "workspace-target"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("second Claim() error = %v, want ErrSessionNotFound", err)
	}
}

type fakeRepository struct {
	sessions      map[string]*Session
	tokens        map[string]*Session
	createdName   string
	createdSlug   string
	claimedTarget string
}

// newFakeRepository 创建游客会话内存仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		sessions: make(map[string]*Session),
		tokens:   make(map[string]*Session),
	}
}

// Create 保存内存游客工作空间与会话
func (r *fakeRepository) Create(_ context.Context, session *Session, name, slug string) error {
	copy := cloneSession(session)
	r.sessions[session.ID] = copy
	r.tokens[string(session.TokenHash)] = copy
	r.createdName = name
	r.createdSlug = slug
	return nil
}

// FindByTokenHash 按摘要查询内存游客会话
func (r *fakeRepository) FindByTokenHash(_ context.Context, tokenHash []byte) (*Session, error) {
	session, ok := r.tokens[string(tokenHash)]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

// ReserveLinkSlot 模拟原子占用游客链接配额
func (r *fakeRepository) ReserveLinkSlot(
	_ context.Context,
	sessionID string,
	maxLinks int,
	now time.Time,
) error {
	session, ok := r.sessions[sessionID]
	if !ok || !session.ExpiresAt.After(now) || int(session.LinkCount) >= maxLinks {
		return ErrLinkLimitReached
	}
	session.LinkCount++
	return nil
}

// ReleaseLinkSlot 模拟归还游客链接配额
func (r *fakeRepository) ReleaseLinkSlot(_ context.Context, sessionID string, _ time.Time) error {
	if session, ok := r.sessions[sessionID]; ok && session.LinkCount > 0 {
		session.LinkCount--
	}
	return nil
}

// CleanupExpired 模拟过期游客数据清理
func (r *fakeRepository) CleanupExpired(_ context.Context, _ time.Time, _ int) (int, error) {
	return 0, nil
}

// Claim 模拟将游客会话消费到目标工作空间
func (r *fakeRepository) Claim(
	_ context.Context,
	tokenHash []byte,
	targetWorkspaceID string,
	now time.Time,
) (*ClaimResult, error) {
	session, ok := r.tokens[string(tokenHash)]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if !session.ExpiresAt.After(now) {
		return nil, ErrSessionExpired
	}
	delete(r.tokens, string(tokenHash))
	delete(r.sessions, session.ID)
	r.claimedTarget = targetWorkspaceID
	return &ClaimResult{
		Links:     1,
		Clicks:    2,
		CacheKeys: []ClaimedLink{{DomainID: "domain-1", Slug: "trial"}},
	}, nil
}

// cloneSession 复制游客会话，避免测试仓库暴露内部切片
func cloneSession(session *Session) *Session {
	copy := *session
	copy.TokenHash = append([]byte(nil), session.TokenHash...)
	return &copy
}
