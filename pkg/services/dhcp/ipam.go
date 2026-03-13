package dhcp

import (
	"math/big"
	"net"
	"net/netip"
)

type IPAM interface {
	// 返回下一个可用的 IP 地址，可以是顺序的或随机的
	NextFreeAddress(identifier string) *netip.Addr
	// 检查 IP 地址是否被使用（还应检查 IP 是否在指定范围和子网内）
	// 可以选择检查 IP 地址是否可 ping
	// `identifier` 也可能被提供给可能请求已经获取的地址的设备
	IsIPFree(addr netip.Addr, identifier *string) bool
	// 获取作用域的子网掩码
	GetSubnetMask() net.IPMask
	// 将 IP 标记为已使用
	UseIP(addr netip.Addr, identifier string)
	// 将 IP 标记为未使用
	FreeIP(ip netip.Addr)
	// 当作用域更新时更新配置
	UpdateConfig(s *Scope) error
	// 可用 IP 数量（排除任何排除项）
	UsableSize() *big.Int
}
