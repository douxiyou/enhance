package dhcp

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"time"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	"douxiyou.com/enhance/pkg/services/dhcp/types"
	"douxiyou.com/enhance/pkg/storage"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type ScopeDNS struct {
	Zone              string   `json:"zone"`
	Search            []string `json:"search"`
	AddZoneInHostname bool     `json:"addZoneInHostname"`
}

type Scope struct {
	ipam IPAM
	inst services.Instance
	DNS  *ScopeDNS

	IPAM    InternalIPAM
	service *Service
	log     *zap.Logger
	// CIDR of the scope e.g. 192.168.1.0/24
	cidr netip.Prefix
	// Subnet mask of the scope e.g. 255.255.255.0
	mask net.IPMask
	// Name of the scope e.g. "default"
	Name string

	Options []*types.DHCPOption
	TTL     int64
	Default bool
	Prefix  string
}

func (r *Service) NewScope(name string) *Scope {
	return &Scope{
		Name:    name,
		inst:    r.i,
		service: r,
		TTL:     int64((7 * 24 * time.Hour).Seconds()),
		log:     r.log.With(zap.String("scope", name)),
		DNS:     &ScopeDNS{},
		IPAM:    InternalIPAM{},
		Default: true,
	}
}

func (s *Service) scopeFromViper() (*Scope, error) {
	scopeConfig := config.GetGlobalConfig().Dhcp.Scope
	scope := s.NewScope(scopeConfig.Name)
	scope.mask = net.IPMask(scopeConfig.Mask)
	start, err := netip.ParseAddr(scopeConfig.RangeStart)
	if err != nil {
		return nil, errors.Wrap(err, "解析 'range_start' 失败")
	}
	end, err := netip.ParseAddr(scopeConfig.RangeEnd)
	if err != nil {
		return nil, errors.Wrap(err, "解析 'range_end' 失败")
	}
	scope.IPAM = InternalIPAM{
		Type:       InternalIPAMType,
		Start:      start,
		End:        end,
		shouldPing: scopeConfig.ShouldPing,
	}
	scope.TTL = scopeConfig.TTL
	s.log.Debug("配置文件:::scope config", zap.Any("config", scopeConfig))
	cidr, err := netip.ParsePrefix(scopeConfig.SubnetCIDR)
	if err != nil {
		return nil, errors.Wrap(err, "解析 'subnet_cidr' 失败")
	}
	scope.cidr = cidr
	ipamInst, err := scope.ipamType()
	if err != nil {
		return nil, errors.Wrap(err, "创建 IPAM 失败")
	}
	scope.ipam = ipamInst
	return scope, nil
}

func (s *Scope) ipamType() (IPAM, error) {
	switch s.IPAM.Type {
	case InternalIPAMType:
		fallthrough
	default:
		return NewInternalIPAM(s.service, s)
	}
}

func (s *Service) findScopeForRequest(req *Request4) *Scope {
	var match *Scope
	longestBits := 0
	const dhcpRelayBias = 1
	const clientIPBias = 2
	scope := s.scope
	clientIPMatchBits := scope.match(req.ClientIPAddr)
	if clientIPMatchBits > -1 && clientIPMatchBits+clientIPBias > longestBits {
		req.log.Debug("selected scope based on client IP", zap.String("scope", scope.Name))
		match = scope
		longestBits = clientIPMatchBits + clientIPBias
	}
	gatewayMatchBits := scope.match(req.GatewayIPAddr)
	if gatewayMatchBits > -1 && gatewayMatchBits+dhcpRelayBias > longestBits {
		req.log.Debug("selected scope based on cidr match (gateway IP)", zap.String("scope", scope.Name))
		match = scope
		longestBits = gatewayMatchBits + dhcpRelayBias
	}
	localMatchBits := scope.match(net.ParseIP(req.LocalIP()))
	if localMatchBits > -1 && localMatchBits > longestBits {
		req.log.Debug("selected scope based on cidr match (instance/interface IP)", zap.String("scope", scope.Name))
		match = scope
		longestBits = localMatchBits
	}
	if match == nil && scope.Default {
		req.log.Debug("selected scope based on default flag", zap.String("scope", scope.Name))
		match = scope
	}
	if match != nil {
		req.log.Debug("final scope selection", zap.String("scope", match.Name))
	}
	return match
}

func (s *Scope) match(peer net.IP) int {
	ip, err := netip.ParseAddr(peer.String())
	if err != nil {
		s.log.Warn("failed to parse client ip", zap.Error(err))
		return -1
	}
	if s.cidr.Contains(ip) {
		return s.cidr.Bits()
	}
	return -1
}

func (s *Scope) leaseFor(req *Request4) *Lease {
	ident := s.service.DeviceIdentifier(req.DHCPv4)
	lease := s.service.NewLease(ident)
	lease.Hostname = req.HostName()

	lease.scope = s
	lease.ScopeKey = s.Name
	lease.setLeaseIP(req)
	req.log.Info("creating new DHCP lease", zap.String("ip", lease.Address), zap.String("identifier", ident))
	return lease
}

func (s *Scope) Put(ctx context.Context, expiry int64, opts ...storage.OpOption) error {
	raw, err := json.Marshal(&s)
	if err != nil {
		return err
	}

	leaseKey := s.inst.KV().Key(
		types.KeyService,
		types.KeyScopes,
		s.Name,
	)
	_, err = s.inst.KV().Put(
		ctx,
		leaseKey.String(),
		string(raw),
		opts...,
	)
	if err != nil {
		return err
	}
	return nil
}
