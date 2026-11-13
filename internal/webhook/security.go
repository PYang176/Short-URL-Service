package webhook

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ValidateEndpointURL 校验回调协议、主机和 SSRF 风险
func ValidateEndpointURL(
	ctx context.Context,
	raw string,
	allowPrivate bool,
) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return "", fmt.Errorf("%w: URL 格式无效", ErrInvalidInput)
	}
	if allowPrivate {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("%w: URL 只允许 HTTP 或 HTTPS", ErrInvalidInput)
		}
	} else if parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: 公网 Webhook 必须使用 HTTPS", ErrUnsafeTarget)
	}
	if parsed.Port() != "" && (parsed.Port() != "80" && parsed.Port() != "443" && !allowPrivate) {
		return "", fmt.Errorf("%w: 公网 Webhook 只允许 80/443 端口", ErrUnsafeTarget)
	}
	if err := validateHost(ctx, parsed.Hostname(), allowPrivate); err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

// NewHTTPClient 创建禁止重定向并在公网模式阻止私网连接的客户端
func NewHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if allowPrivate {
			return dialer.DialContext(ctx, network, address)
		}
		ips, err := resolvePublicIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			connection, err := dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(ip.String(), port),
			)
			if err == nil {
				return connection, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("%w: 连接已校验的 Webhook 地址失败: %v", ErrUnsafeTarget, lastErr)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// validateHost 阻止公网模式访问回环、私网和链路本地地址
func validateHost(ctx context.Context, host string, allowPrivate bool) error {
	if allowPrivate {
		return nil
	}
	_, err := resolvePublicIPs(ctx, host)
	return err
}

// resolvePublicIPs 解析主机并拒绝任一非公网地址
func resolvePublicIPs(ctx context.Context, host string) ([]net.IP, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%w: 无法解析 webhook 主机: %v", ErrUnsafeTarget, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("%w: webhook 主机没有地址", ErrUnsafeTarget)
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("%w: webhook 主机解析到非公网地址", ErrUnsafeTarget)
		}
	}
	return ips, nil
}

// isPublicIP 判断地址是否可安全用于公网 Webhook
func isPublicIP(ip net.IP) bool {
	return ip != nil &&
		ip.IsGlobalUnicast() &&
		!ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}
