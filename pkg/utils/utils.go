package utils

import (
	"fmt"
	"net/netip"
	"strings"
)

// ParseIP 安全解析 IP 地址，返回有效地址或空
func ParseIP(ipStr string) netip.Addr {
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() {
		return netip.Addr{}
	}
	return addr
}

// ParseCIDR 安全解析 CIDR 网段，返回有效网段或空
func ParseCIDR(cidrStr string) netip.Prefix {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidrStr))
	if err != nil || !prefix.Addr().IsValid() {
		return netip.Prefix{}
	}
	return prefix
}

// IsIPInCIDR 判断 IP 是否在指定 CIDR 网段内
func IsIPInCIDR(ipStr, cidrStr string) bool {
	ip := ParseIP(ipStr)
	if !ip.IsValid() {
		return false
	}
	prefix := ParseCIDR(cidrStr)
	if !prefix.Addr().IsValid() {
		return false
	}
	return prefix.Contains(ip)
}

// IPv4MaskLenToStr 将 IPv4 掩码长度转为子网掩码字符串（如 24 → 255.255.255.0）
func IPv4MaskLenToStr(maskLen int) string {
	if maskLen < 0 || maskLen > 32 {
		return ""
	}
	mask := [4]byte{}
	remaining := maskLen
	for i := 0; i < 4; i++ {
		bits := 8
		if remaining < 8 {
			bits = remaining
		}
		if bits > 0 {
			mask[i] = 0xff << (8 - bits)
		}
		remaining -= bits
		if remaining <= 0 {
			break
		}
	}
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
