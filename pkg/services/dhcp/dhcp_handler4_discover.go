package dhcp

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func (s *Service) HandleDHCPDiscover4(req *Request4) *dhcpv4.DHCPv4 {
	match := s.FindLease(req)
	if match == nil {
		scope := s.findScopeForRequest(req)
		if scope == nil {
			req.log.Info("no scope found")
			return nil
		}
		req.log.Debug("found scope for new lease", zap.String("scope", scope.Name))
		match = scope.leaseFor(req)
		if match == nil {
			return nil
		}
		err := match.Put(req.Context, int64(viper.GetInt("dhcp.lease_negotiate_timeout")))
		if err != nil {
			req.log.Warn("failed to update lease during discover creation", zap.Error(err))
		}
	} else {
		err := match.Put(req.Context, match.scope.TTL)
		if err != nil {
			req.log.Warn("failed to update lease during discover", zap.Error(err))
		}
	}
	rep := match.createReply(req)
	rep.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	return rep
}
