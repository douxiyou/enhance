package dhcp

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"net"
	"net/netip"
	"strings"
	"time"

	"douxiyou.com/enhance/pkg/services"
	"douxiyou.com/enhance/pkg/services/dhcp/types"
	"douxiyou.com/enhance/pkg/storage"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/rfc1035label"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Lease struct {
	inst  services.Instance
	scope *Scope
	log   *zap.Logger

	Identifier string `json:"-"`

	Address          string `json:"address"`
	Hostname         string `json:"hostname"`
	AddressLeaseTime string `json:"addressLeaseTime,omitempty"`
	ScopeKey         string `json:"scopeKey"`
	DNSZone          string `json:"dnsZone,omitempty"`
	Expiry           int64  `json:"expiry"`
	Description      string `json:"description"`

	badgerKey string
}

func (s *Service) FindLease(req *Request4) *Lease {
	lease, ok := s.leases.GetWithPrefix(s.DeviceIdentifier(req.DHCPv4))
	if !ok {
		s.log.Debug("根据请求identifier未找到lease", zap.String("identifier", s.DeviceIdentifier(req.DHCPv4)))
		return nil
	}

	expectedScope := s.findScopeForRequest(req)
	// 此处scope不应该出现不符合的情况,因为我们只有一个scope,并没有多scope的情况
	if expectedScope != nil && lease.scope != expectedScope {
		lease.scope = expectedScope
		lease.ScopeKey = expectedScope.Name
		lease.setLeaseIP(req)
		lease.log.Info("由于请求scope变更，重新分配lease地址", zap.String("newIP", lease.Address))
		err := lease.Put(req.Context, lease.scope.TTL)
		if err != nil {
			s.log.Warn("更新重新分配IP的lease失败", zap.Error(err))
		}
	}
	return lease
}

func (s *Service) NewLease(identifier string) *Lease {
	return &Lease{
		inst:       s.i,
		Identifier: identifier,
		log:        s.log.With(zap.String("identifier", identifier)),
		Expiry:     0,
	}
}

func (l *Lease) setLeaseIP(req *Request4) {
	requestedIP := req.RequestedIPAddress()
	if requestedIP != nil {
		req.log.Debug("检查请求的 IP", zap.String("ip", requestedIP.String()))
		ip, _ := netip.AddrFromSlice(requestedIP)
		if l.scope.ipam.IsIPFree(ip, &l.Identifier) {
			req.log.Debug("请求的 IP 是空闲的", zap.String("ip", requestedIP.String()))
			l.Address = requestedIP.String()
			l.scope.ipam.UseIP(ip, l.Identifier)
			return
		}
	}
	ip := l.scope.ipam.NextFreeAddress(l.Identifier)
	if ip == nil {
		return
	}
	req.log.Debug("使用 IPAM 中的下一个空闲 IP", zap.String("ip", ip.String()))
	l.Address = ip.String()
	l.scope.ipam.UseIP(*ip, l.Identifier)
}

func (s *Service) leaseFromKV(raw *storage.KeyValue) (*Lease, error) {
	prefix := s.i.KV().Key(
		types.KeyService,
		types.KeyLeases,
	).Prefix(true).String()
	identifier := strings.TrimPrefix(string(raw.Key), prefix)
	l := s.NewLease(identifier)
	err := json.Unmarshal(raw.Value, &l)
	if err != nil {
		return l, err
	}
	l.badgerKey = string(raw.Key)

	l.scope = s.scope
	return l, nil
}

func (l *Lease) IsReservation() bool {
	return l.Expiry == -1
}

func (l *Lease) Delete(ctx context.Context) error {
	leaseKey := l.inst.KV().Key(
		types.KeyService,
		types.KeyLeases,
		l.Identifier,
	)
	_, err := l.inst.KV().Delete(
		ctx,
		leaseKey.String(),
	)
	return err
}

func (l *Lease) Put(ctx context.Context, expiry int64, opts ...storage.OpOption) error {
	if expiry > 0 && !l.IsReservation() {
		l.Expiry = time.Now().Add(time.Duration(expiry) * time.Second).Unix()
	}

	raw, err := json.Marshal(&l)
	if err != nil {
		return err
	}

	leaseKey := l.inst.KV().Key(
		types.KeyService,
		types.KeyLeases,
		l.Identifier,
	)
	_, err = l.inst.KV().Put(
		ctx,
		leaseKey.String(),
		string(raw),
		opts...,
	)
	if err != nil {
		l.log.Warn("保存 lease 失败", zap.String("key", leaseKey.String()))
		return err
	}
	l.log.Debug("保存 lease", zap.Any("key", leaseKey.String()), zap.Any("value", string(raw)))
	return nil
}

func (l *Lease) createReply(req *Request4) *dhcpv4.DHCPv4 {
	rep, err := dhcpv4.NewReplyFromRequest(req.DHCPv4)
	if err != nil {
		req.log.Warn("创建回复失败", zap.Error(err))
		return nil
	}
	rep.UpdateOption(dhcpv4.OptSubnetMask(l.scope.ipam.GetSubnetMask()))
	rep.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Duration(l.scope.TTL * int64(time.Second))))

	if l.AddressLeaseTime != "" {
		pl, err := time.ParseDuration(l.AddressLeaseTime)
		if err != nil {
			req.log.Warn("解析地址租约时长失败，使用默认值", zap.Error(err), zap.String("default", pl.String()))
		} else if pl.Seconds() < 1 {
			req.log.Warn("无效的时长: 小于 1", zap.String("duration", l.AddressLeaseTime))
		} else if pl.Seconds() > math.MaxInt32 {
			req.log.Warn("无效的时长: 时长过长", zap.String("duration", l.AddressLeaseTime))
		} else {
			rep.UpdateOption(dhcpv4.OptIPAddressLeaseTime(pl))
		}
	}

	ip := viper.GetString("instance.ip")
	rep.UpdateOption(dhcpv4.OptDNS(net.ParseIP(ip)))
	if l.scope.DNS != nil {
		rep.UpdateOption(dhcpv4.OptDomainName(l.scope.DNS.Zone))
		if len(l.scope.DNS.Search) > 0 {
			rep.UpdateOption(dhcpv4.OptDomainSearch(&rfc1035label.Labels{Labels: l.scope.DNS.Search}))
		}
	}

	if req.HostName() != l.Hostname {
		l.Hostname = req.HostName()
		err := l.Put(req.Context, l.Expiry)
		if err != nil {
			l.log.Warn("更新主机名的 lease 失败", zap.Error(err))
		}
	}
	if l.Hostname != "" {
		hostname := l.Hostname
		if l.scope.DNS != nil && l.scope.DNS.AddZoneInHostname {
			fqdn := strings.Join([]string{l.Hostname, l.scope.DNS.Zone}, ".")
			hostname = fqdn
		}
		rep.UpdateOption(dhcpv4.OptHostName(strings.TrimSuffix(hostname, ".")))
	}

	rep.ServerIPAddr = net.ParseIP(ip)
	rep.UpdateOption(dhcpv4.OptServerIdentifier(rep.ServerIPAddr))
	rep.YourIPAddr = net.ParseIP(l.Address)

	for _, opt := range l.scope.Options {
		finalVal := make([]byte, 0)
		if opt == nil || opt.Tag == nil && opt.TagName == "" {
			continue
		}
		if opt.TagName != "" {
			tag, ok := types.TagMap[types.OptionTagName(opt.TagName)]
			if !ok {
				req.log.Warn("无效的标签名称", zap.String("tagName", opt.TagName))
				continue
			}
			opt.Tag = &tag
		}

		if opt.Value != nil {
			finalVal = []byte(*opt.Value)
			if _, ok := types.IPTags[*opt.Tag]; ok {
				i := net.ParseIP(*opt.Value)
				finalVal = dhcpv4.IPs([]net.IP{i}).ToBytes()
			}
		}

		if len(opt.Value64) > 0 {
			values64 := make([]byte, 0)
			for _, v := range opt.Value64 {
				va, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					req.log.Warn("转换 base64 值到字节失败", zap.Error(err))
					continue
				}
				values64 = append(values64, va...)
			}
			finalVal = values64
		}
		if len(opt.ValueHex) > 0 {
			valuesHex := make([]byte, 0)
			for _, v := range opt.ValueHex {
				va, err := hex.DecodeString(v)
				if err != nil {
					req.log.Warn("转换 hex 值到字节失败", zap.Error(err))
					continue
				}
				valuesHex = append(valuesHex, va...)
			}
			finalVal = valuesHex
		}
		dopt := dhcpv4.OptGeneric(dhcpv4.GenericOptionCode(*opt.Tag), finalVal)
		rep.UpdateOption(dopt)
		if dopt.Code.Code() == uint8(dhcpv4.OptionBootfileName) {
			rep.BootFileName = dhcpv4.GetString(dopt.Code, rep.Options)
		}
	}
	return rep
}
