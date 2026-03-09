package dhcp

import (
	"context"

	"douxiyou.com/enhance/pkg/services"
	"douxiyou.com/enhance/pkg/services/dhcp/oui"
	"douxiyou.com/enhance/pkg/storage/watcher"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
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

func init() {
	services.RegisterService("dhcp", func(i services.Instance) services.Service {
		return NewDhcpService(i)
	})
}
func NewDhcpService(i services.Instance) services.Service {
	s := &Service{
		i:   i,
		ctx: i.Context(),
		log: i.Log(),
	}
	s.scope = s.scopeFromViper()
}
func (r *Service) DeviceIdentifier(m *dhcpv4.DHCPv4) string {
	return m.ClientHWAddr.String()
}
