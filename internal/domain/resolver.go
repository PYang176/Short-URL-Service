package domain

import (
	"context"
	"net"
)

// Resolver 定义 TXT 和 CNAME 查询能力
type Resolver interface {
	// LookupTXT 查询 TXT 记录
	LookupTXT(ctx context.Context, hostname string) ([]string, error)
	// LookupCNAME 查询 CNAME 记录
	LookupCNAME(ctx context.Context, hostname string) (string, error)
}

// DNSResolver 使用系统 DNS 查询
type DNSResolver struct{}

// LookupTXT 查询 TXT 记录
func (DNSResolver) LookupTXT(ctx context.Context, hostname string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, hostname)
}

// LookupCNAME 查询 CNAME 记录
func (DNSResolver) LookupCNAME(ctx context.Context, hostname string) (string, error) {
	return net.DefaultResolver.LookupCNAME(ctx, hostname)
}
