package dhcp

import (
	"math/big"
	"net"
	"net/netip"
	"strconv"
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

	ipf        sync.Mutex
	log        *zap.Logger
	service    *Service
	scope      *Scope
	SubnetCIDR netip.Prefix

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
	sub, err := netip.ParsePrefix(s.SubnetCIDR)
	if err != nil {
		return errors.Wrap(err, "failed to parse scope cidr")
	}
	start, err := netip.ParseAddr(s.IPAM["range_start"])
	if err != nil {
		return errors.Wrap(err, "failed to parse 'range_start'")
	}
	end, err := netip.ParseAddr(s.IPAM["range_end"])
	if err != nil {
		return errors.Wrap(err, "failed to parse 'range_end'")
	}
	i.SubnetCIDR = sub
	i.Start = start
	i.End = end
	sp := s.IPAM["should_ping"]
	if sp != "" {
		shouldPing, err := strconv.ParseBool(sp)
		if err != nil {
			return err
		}
		i.shouldPing = shouldPing
	}
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
		if !i.SubnetCIDR.Contains(currentIP) {
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
	_, cidr, err := net.ParseCIDR(i.SubnetCIDR.String())
	if err != nil {
		// 这种情况绝不可能发生，因为 CIDR 的验证工作是在构造函数中完成的。
		panic(err)
	}
	return cidr.Mask
}

func (i *InternalIPAM) UsableSize() *big.Int {
	ips := iprange.New(i.Start.AsSlice(), i.End.AsSlice())
	return ips.Size()
}
