package domain

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestServiceAddAndMockVerify 验证域名规范化、模拟验证和授权
func TestServiceAddAndMockVerify(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, fakeResolver{}, verifyModeMock, "links.localhost")
	service.now = func() time.Time {
		return time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	}

	added, err := service.Add(context.Background(), "workspace-1", "HTTPS://Go.Example.com/path")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if added.Hostname != "go.example.com" || added.VerificationToken == "" {
		t.Fatalf("Add() = %+v", added)
	}

	verified, err := service.Verify(context.Background(), "workspace-1", added.ID)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Status != StatusVerified || verified.LastVerifiedAt == nil {
		t.Fatalf("Verify() = %+v", verified)
	}

	hostname, err := service.AuthorizeLinkDomain(context.Background(), "workspace-1", added.ID)
	if err != nil || hostname != "go.example.com" {
		t.Fatalf("AuthorizeLinkDomain() = %q, %v", hostname, err)
	}
}

// TestServiceDNSVerification 验证 TXT 和 CNAME 必须同时匹配
func TestServiceDNSVerification(t *testing.T) {
	repo := newFakeRepository()
	resolver := &fakeResolver{}
	service := NewService(repo, resolver, verifyModeDNS, "links.localhost")
	added, err := service.Add(context.Background(), "workspace-1", "go.example.com")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	resolver.txt = []string{added.VerificationToken}
	resolver.cname = "links.localhost."

	if _, err := service.Verify(context.Background(), "workspace-1", added.ID); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	second, err := service.Add(context.Background(), "workspace-1", "bad.example.com")
	if err != nil {
		t.Fatalf("Add() second error = %v", err)
	}
	resolver.cname = "wrong.localhost."
	if _, err := service.Verify(context.Background(), "workspace-1", second.ID); !errors.Is(err, ErrVerificationFailed) {
		t.Fatalf("Verify() wrong CNAME error = %v", err)
	}
}

// TestServiceRejectsInvalidDomain 验证非法域名输入
func TestServiceRejectsInvalidDomain(t *testing.T) {
	service := NewService(newFakeRepository(), fakeResolver{}, verifyModeMock, "links.localhost")
	for _, hostname := range []string{"", "localhost", "127.0.0.1", "-bad.example.com"} {
		if _, err := service.Add(context.Background(), "workspace-1", hostname); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("Add(%q) error = %v, want ErrInvalidDomain", hostname, err)
		}
	}
}

type fakeRepository struct {
	mu      sync.Mutex
	domains map[string]*Domain
	links   int
}

// newFakeRepository 创建内存域名仓库
func newFakeRepository() *fakeRepository {
	return &fakeRepository{domains: make(map[string]*Domain)}
}

// Create 保存域名副本
func (r *fakeRepository) Create(_ context.Context, value *Domain) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.domains {
		if existing.Hostname == value.Hostname {
			return ErrDomainExists
		}
	}
	copy := *value
	r.domains[value.ID] = &copy
	return nil
}

// ListByWorkspace 返回工作空间域名
func (r *fakeRepository) ListByWorkspace(_ context.Context, workspaceID string) ([]Domain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Domain, 0)
	for _, value := range r.domains {
		if value.WorkspaceID == workspaceID && value.DeletedAt == nil {
			result = append(result, *value)
		}
	}
	return result, nil
}

// FindByID 按工作空间和 ID 返回域名
func (r *fakeRepository) FindByID(_ context.Context, workspaceID, id string) (*Domain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.domains[id]
	if !ok || value.WorkspaceID != workspaceID || value.DeletedAt != nil {
		return nil, ErrNotFound
	}
	copy := *value
	return &copy, nil
}

// FindVerifiedByHostname 返回已验证 Host
func (r *fakeRepository) FindVerifiedByHostname(_ context.Context, hostname string) (*Domain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, value := range r.domains {
		if value.Hostname == hostname && value.Status == StatusVerified && value.DeletedAt == nil {
			copy := *value
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

// UpdateVerification 覆盖验证状态
func (r *fakeRepository) UpdateVerification(_ context.Context, value *Domain) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.domains[value.ID]; !ok {
		return ErrNotFound
	}
	copy := *value
	r.domains[value.ID] = &copy
	return nil
}

// SoftDelete 标记域名已删除
func (r *fakeRepository) SoftDelete(_ context.Context, workspaceID, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.domains[id]
	if !ok || value.WorkspaceID != workspaceID {
		return ErrNotFound
	}
	value.DeletedAt = &now
	return nil
}

// CountLinks 返回测试链接数量
func (r *fakeRepository) CountLinks(context.Context, string) (int, error) {
	return r.links, nil
}

type fakeResolver struct {
	txt   []string
	cname string
}

// LookupTXT 返回测试 TXT 记录
func (r fakeResolver) LookupTXT(context.Context, string) ([]string, error) {
	return r.txt, nil
}

// LookupCNAME 返回测试 CNAME 记录
func (r fakeResolver) LookupCNAME(context.Context, string) (string, error) {
	return r.cname, nil
}
