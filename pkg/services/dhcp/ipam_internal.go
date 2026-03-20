package dhcp

import (
	"math/big"
	"net"
	"net/netip"
	"sync"

	"github.com/netdata/go.d.plugin/pkg/iprange"
	"go.uber.org/zap"
)

const InternalIPAMType = "internal"

type InternalIPAM struct {
	Type  string
	Start netip.Addr
	End   netip.Addr

	ipf     sync.Mutex
	log     *zap.Logger
	service *Service
	scope   *Scope

	shouldPing bool
	scopeLock  sync.Mutex
}

func NewInternalIPAM(service *Service, s *Scope) (*InternalIPAM, error) {
	ipam := &InternalIPAM{
		log:     service.log.With(zap.String("ipam", "internal")),
		service: service,
		scope:   s,
		ipf:     sync.Mutex{},
	}

	err := ipam.UpdateConfig(s)
	if err != nil {
		return nil, err
	}
	return ipam, nil
}

func (i *InternalIPAM) UpdateConfig(s *Scope) error {
	start, err := netip.ParseAddr(s.IPAM.RangeStart)
	if err != nil {
		return errors.Wrap(err, "解析 'range_start' 失败")
	}
	end, err := netip.ParseAddr(s.IPAM.RangeEnd)
	if err != nil {
		return errors.Wrap(err, "解析 'range_end' 失败")
	}
	i.Start = start
	i.End = end
	i.shouldPing = s.IPAM.ShouldPing
	return nil
}

func (i *InternalIPAM) NextFreeAddress(identifier string) *netip.Addr {
	i.ipf.Lock()
	defer i.ipf.Unlock()
	currentIP := i.Start
	for i.End.Compare(currentIP) != -1 {
		i.log.Debug("检查可用 IP", zap.String("ip", currentIP.String()))
		if !i.scope.cidr.Contains(currentIP) {
			i.log.Debug("CIDR 不包含当前IP:", zap.String("ip", currentIP.String()))
			break
		}
		if i.IsIPFree(currentIP, &identifier) {
			return &currentIP
		}
		currentIP = currentIP.Next()
	}
	i.log.Warn("没有可用的 IP 地址", zap.String("lastIp", currentIP.String()))
	return nil
}

func (i *InternalIPAM) FreeIP(ip netip.Addr) {
}

func (i *InternalIPAM) UseIP(ip netip.Addr, identifier string) {
}

func (i *InternalIPAM) IsIPFree(ip netip.Addr, identifier *string) bool {
	i.scopeLock.Lock()
	if identifier != nil {
		l, ok := i.service.leases.GetOK(*identifier)
		if ok && l.Address == ip.String() {
			i.log.Debug("允许", zap.String("ip", ip.String()), zap.String("reason", "lease 的现有 IP"))
			i.scopeLock.Unlock()
			return true
		}
	}
	for _, l := range i.service.leases.Iter() {
		if l.Address == ip.String() {
			i.log.Debug("丢弃", zap.String("ip", ip.String()), zap.String("reason", "已使用 (内存中)"))
			i.scopeLock.Unlock()
			return false
		}
	}
	i.scopeLock.Unlock()
	if i.Start.Compare(ip) == 1 {
		i.log.Debug("丢弃", zap.String("ip", ip.String()), zap.String("reason", "在起始地址之前"))
		return false
	}
	if i.End.Compare(ip) == -1 {
		i.log.Debug("丢弃", zap.String("ip", ip.String()), zap.String("reason", "在结束地址之后"))
		return false
	}
	for _, l := range i.service.leases.Iter() {
		if l.ScopeKey != i.scope.Name {
			continue
		}
		if l.Address != ip.String() {
			continue
		}
		if identifier != nil && l.Identifier == *identifier {
			i.UseIP(ip, *identifier)
			i.log.Debug("允许", zap.String("ip", ip.String()), zap.String("reason", "现有匹配的 lease"))
			return true
		}
		i.log.Debug("丢弃", zap.String("ip", ip.String()), zap.String("reason", "现有 lease"))
		return false
	}

	i.log.Debug("允许", zap.String("ip", ip.String()), zap.String("reason", "空闲"))
	return true
}

func (i *InternalIPAM) GetSubnetMask() net.IPMask {
	return i.scope.mask
}

func (i *InternalIPAM) UsableSize() *big.Int {
	ips := iprange.New(i.Start.AsSlice(), i.End.AsSlice())
	return ips.Size()
}
