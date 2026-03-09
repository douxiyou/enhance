package dhcp

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
)

func (s *Service) HandleDHCPRequest4(req *Request4) *dhcpv4.DHCPv4 {
	match := s.FindLease(req)

	if match == nil {
		scope := s.findScopeForRequest(req)
		if scope == nil {
			return nil
		}
		req.log.Debug("found scope for new lease", zap.String("scope", scope.Name))
		match = scope.leaseFor(req)
		if match == nil {
			return nil
		}
	}

	err := match.Put(req.Context, match.scope.TTL)
	if err != nil {
		req.log.Warn("failed to put dhcp lease", zap.Error(err))
	}

	rep := match.createReply(req)
	rep.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	return rep
}
