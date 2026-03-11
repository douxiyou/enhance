package dhcp

import (
	"context"
	"fmt"
	"net"
	"strings"

	"douxiyou.com/enhance/pkg/config"
	"douxiyou.com/enhance/pkg/services"
	"douxiyou.com/enhance/pkg/services/dhcp/oui"
	"douxiyou.com/enhance/pkg/services/dhcp/types"
	"douxiyou.com/enhance/pkg/storage/watcher"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
)

type Service struct {
	i      services.Instance
	ctx    context.Context
	scope  *Scope
	leases *watcher.Watcher[*Lease]
	s4     *handler4
	log    *zap.Logger

	oui *oui.OuiDb
}

func (s *Service) Name() string {
	return "dhcp"
}

func init() {
	services.RegisterService("dhcp", func(i services.Instance) services.Service {
		return NewDhcpService(i)
	})
}
func NewDhcpService(i services.Instance) *Service {
	s := &Service{
		i:   i,
		ctx: i.Context(),
		log: i.Log(),
	}
	scope, err := s.scopeFromViper()
	// TODO: 这个位置我 觉得不是很合适,当出现错误时,应该返回错误,而不是返回默认值
	if err != nil {
		s.log.Error("failed to create scope from viper", zap.Error(err))
		return nil
	}
	s.scope = scope
	s.leases = watcher.New(
		func(kv *mvccpb.KeyValue) (*Lease, error) {
			s, err := s.leaseFromKV(kv)
			if err != nil {
				return nil, err
			}
			return s, nil
		},
		s.i.KV(),
		s.i.KV().Key(
			types.KeyService,
			types.KeyLeases,
		).Prefix(true),
	)
	s.s4 = &handler4{
		service: s,
	}
	return s
}
func (s *Service) Handler4() *handler4 {
	return s.s4
}
func (s *Service) Start(ctx context.Context) error {
	// s.scope.Start(ctx) // 这个作用域我没有使用etcd,所以也不会涉及watcher
	s.leases.Start(ctx)
	err := s.initHandler4()
	if err != nil {
		s.log.Fatal("failed to init handler4", zap.Error(err))
		return err
	}
	go func() {
		err := s.startServer4()
		if err != nil {
			s.log.Warn("failed to listen", zap.Error(err))
		}
	}()
	return nil
}
func (s *Service) Stop(ctx context.Context) error {
	s.leases.Stop(ctx)
	// s.scope.Stop(ctx)
	return nil
}

func (s *Service) initHandler4() error {
	dhcpConig := config.GetGlobalConfig().Dhcp
	laddr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: 67,
	}
	ifName := dhcpConig.Interface
	udpConn, err := server4.NewIPv4UDPConn(ifName, laddr)
	if err != nil {
		return err
	}
	s.s4.pc = ipv4.NewPacketConn(udpConn)
	var ifi *net.Interface
	if ifName != "" {
		ifi, err = net.InterfaceByName(ifName)
		if err != nil {
			return fmt.Errorf("DHCPv4: Listen could not find interface %s: %v", ifName, err)
		}
		s.s4.iface = *ifi
	} else {
		// 当未绑定到接口时，我们需要每个数据包中的信息来了解它是从哪个接口进来的。
		s.log.Warn("DHCPv4: Listen not bound to any interface, setting ControlMessage to get interface information from each packet")
		err = s.s4.pc.SetControlMessage(ipv4.FlagInterface, true)
		if err != nil {
			return err
		}
	}

	if laddr.IP.IsMulticast() {
		err = s.s4.pc.JoinGroup(ifi, laddr)
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) startServer4() error {
	s.log.Info("starting DHCP Server", zap.Int("port", 67), zap.String("interface", s.s4.iface.Name))
	err := s.s4.Serve()
	if !isErrNetClosing(err) {
		return err
	}
	return nil
}

var useOfClosedErrMsg = "use of closed network connection"

// isErrNetClosing checks whether is an ErrNetClosing error
func isErrNetClosing(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), useOfClosedErrMsg)
}
func (r *Service) DeviceIdentifier(m *dhcpv4.DHCPv4) string {
	return m.ClientHWAddr.String()
}
