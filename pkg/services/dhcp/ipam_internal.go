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
	i.Start = s.IPAM.Start
	i.End = s.IPAM.End
	i.shouldPing = s.IPAM.shouldPing
	return nil
}

func (i *InternalIPAM) NextFreeAddress(identifier string) *netip.Addr {
	i.ipf.Lock()
	defer i.ipf.Unlock()
	currentIP := i.Start
	for i.End.Compare(currentIP) != -1 {
		i.log.Debug("checking for free IP", zap.String("ip", currentIP.String()))
		if !i.scope.cidr.Contains(currentIP) {
			i.log.Debug("CIDR 不包含当前IP:", zap.String("ip", currentIP.String()))
			break
		}
		if i.IsIPFree(currentIP, &identifier) {
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
	if i.Start.Compare(ip) == 1 {
		i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "before started"))
		return false
	}
	if i.End.Compare(ip) == -1 {
		i.log.Debug("discarding", zap.String("ip", ip.String()), zap.String("reason", "after end"))
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

func (i *InternalIPAM) UsableSize() *big.Int {
	ips := iprange.New(i.Start.AsSlice(), i.End.AsSlice())
	return ips.Size()
}
