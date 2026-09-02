// HTTP 请求安全判定 / HTTP Request Security Decision
// 功能：统一直连 TLS 与可信反向代理链路的 HTTPS 判定
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-26 18:21:07
// Package httpx 提供请求级别的公共判断函数。
package httpx

import (
	"net"
	"net/http"
	"strings"
)

// IsSecureRequest 判定当前请求是否应按 HTTPS 口径发放安全 Cookie。
// 规则：若请求已直连 TLS，则视为 HTTPS；
// 若无 TLS，则仅当来源 IP 在可信代理白名单内且 X-Forwarded-Proto == "https" 时视为 HTTPS。
func IsSecureRequest(r *http.Request, trustedProxies []*net.IPNet) bool {
	if r.TLS != nil {
		return true
	}
	if len(trustedProxies) == 0 {
		return false
	}
	ip := requestIP(r)
	if ip == nil {
		return false
	}
	for _, n := range trustedProxies {
		if n != nil && n.Contains(ip) {
			return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
		}
	}
	return false
}

func requestIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip := net.ParseIP(r.RemoteAddr)
		if ip != nil {
			return ip
		}
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return ip
}
