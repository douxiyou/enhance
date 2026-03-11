package dhcp

import (
	"math/big"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"douxiyou.com/enhance/pkg/services/dhcp/types"
	"github.com/netdata/go.d.plugin/pkg/iprange"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

const InternalIPAMType = "internal"

type InternalIPAM struct {
	Start netip.Addr
	End   netip.Addr

	ipf     sync.Mutex
	log     *zap.Logger
	service *Service
	scope   *Scope

	shouldPing bool
	scopeLock  sync.Locker
}

func NewInternalIPAM(service *Service, s *Scope) (*InternalIPAM, error) {
	ipam := &InternalIPAM{
		log:     service.log.With(zap.String("ipam", "internal")),
		service: service,
		scope:   s,
		ipf:     sync.Mutex{},
	}
	sess, err := concurrency.NewSession(service.i.KV().Client, concurrency.WithContext(service.ctx))
	if err != nil {
		return nil, err
	}
	ipam.scopeLock = concurrency.NewLocker(sess, service.i.KV().Key(
		types.KeyService,
		types.KeyIPAM,
		s.Name,
	).String())

	err = ipam.UpdateConfig(s)
	if err != nil {
		return nil, err
	}
	return ipam, nil
}

func (i *InternalIPAM) UpdateConfig(s *Scope) error {
	start, err := netip.ParseAddr(s.IPAM.RangeStart)
	if err != nil {
		return errors.Wrap(err, "failed to parse 'range_start'")
	}
	end, err := netip.ParseAddr(s.IPAM.RangeEnd)
	if err != nil {
		return errors.Wrap(err, "failed to parse 'range_end'")
	}
	i.Start = start
	i.End = end
	i.shouldPing = s.IPAM.ShouldPing
	return nil
}

// 返回由 `.Start` 和 `.End` 定义的范围内的下一个可用 IP 地址（包括这两个端点）。
// 所返回的地址不会被标记为已使用，因为这取决于函数的调用者
// 如果没有更多可用的 IP 地址，则可能返回 `nil`
func (i *InternalIPAM) NextFreeAddress(identifier string) *netip.Addr {
	i.ipf.Lock()
	defer i.ipf.Unlock()
	currentIP := i.Start
	// Since we start checking at the beginning of the range, check in the loop if we've
	// hit the end and just give up, as the range is full
	for i.End.Compare(currentIP) != -1 {
		i.log.Debug("checking for free IP", zap.String("ip", currentIP.String()))
		// Check if IP is in the correct subnet
		if !i.scope.cidr.Contains(currentIP) {
			break
		}
		if i.IsIPFree(currentIP, &identifier) {
			// Free IP is returned, _not_ marked as used, this the responsibility of the caller
			// to mark the IP as used
			return &currentIP
		}
		currentIP = currentIP.Next()
	}
	i.log.Warn("no more empty IPs left", zap.String("lastIp", currentIP.String()))
	return nil
}

func (i *InternalIPAM) FreeIP(ip netip.Addr) {
}

func (i *InternalIPAM) UseIP(ip netip.Addr, identifier string) {
}

func (i *InternalIPAM) IsIPFree(ip netip.Addr, identifier *string) bool {
	i.scopeLock.Lock()
	if identifier != nil {
		l := i.service.leases.Get(*identifier)
		if l != nil && l.Address == ip.String() {
			i.log.Debug("allowing", zap.String("ip", ip.String()), zap.String("reason", "existing IP of lease"))
			i.scopeLock.Unlock()
			return true
		}
	}
	for _, l := range i.service.leases.Iter() {
		if l.Address == ip.String() {
			i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "used (in memory)"))
			i.scopeLock.Unlock()
			return false
		}
	}
	i.scopeLock.Unlock()
	// IP is less than the start of the range
	if i.Start.Compare(ip) == 1 {
		i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "before started"))
		return false
	}
	// IP is more than the end of the range
	if i.End.Compare(ip) == -1 {
		i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "after end"))
		return false
	}
	// check for existing leases
	for _, l := range i.service.leases.Iter() {
		// Ignore leases from other scopes
		if l.ScopeKey != i.scope.Name {
			continue
		}
		if l.Address != ip.String() {
			continue
		}
		if identifier != nil && l.Identifier == *identifier {
			i.UseIP(ip, *identifier)
			i.log.Debug("allowing", zap.String("ip", ip.String()), zap.String("reason", "existing matching lease"))
			return true
		}
		i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "existing lease"))
		return false
	}

	i.log.Debug("allowing", zap.String("ip", ip.String()), zap.String("reason", "free"))
	return true
}

func (i *InternalIPAM) GetSubnetMask() net.IPMask {
	return i.scope.mask
}

// prefixToSubnetMask 将 netip.Prefix 转换为 IPv4 子网掩码字符串（如 255.255.255.0）
// 仅支持 IPv4，IPv6 返回空字符串
func prefixToSubnetMask(prefix netip.Prefix) string {
	// 仅处理 IPv4
	if !prefix.Addr().Is4() {
		return ""
	}

	maskLen := prefix.Bits()
	// 校验 IPv4 掩码长度范围
	if maskLen < 0 || maskLen > 32 {
		return ""
	}

	// 初始化 4 个字节的掩码（默认全 0）
	mask := [4]byte{}
	for i := 0; i < 4; i++ {
		// 计算当前字节的掩码位数（如 24 位 → 前 3 字节全 1，第 4 字节 0）
		bits := 8
		if maskLen < 8 {
			bits = maskLen
		}
		if bits > 0 {
			mask[i] = 0xff << (8 - bits) // 左移生成掩码字节（如 bits=8 → 0xff，bits=0 → 0x00）
		}
		maskLen -= bits
		if maskLen <= 0 {
			break
		}
	}

	// 将字节数组转为字符串（如 [255,255,255,0] → "255.255.255.0"）
	var parts []string
	for _, b := range mask {
		parts = append(parts, strconv.Itoa(int(b)))
	}
	return strings.Join(parts, ".")
}
func (i *InternalIPAM) UsableSize() *big.Int {
	ips := iprange.New(i.Start.AsSlice(), i.End.AsSlice())
	return ips.Size()
}
