package dhcp

import (
	"net/netip"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
)

func (s *Service) HandleDHCPRelease4(req *Request4) *dhcpv4.DHCPv4 {
	match := s.FindLease(req)
	if match == nil || match.IsReservation() {
		return nil
	}
	ip, err := netip.ParseAddr(match.Address)
	if err != nil {
		req.log.Warn("failed to parse address from lease", zap.Error(err))
	} else {
		match.scope.ipam.FreeIP(ip)
	}
	err = match.Delete(req.Context)
	if err != nil {
		req.log.Warn("failed to delete lease", zap.Error(err))
	} else {
		req.log.Info("deleted lease from release")
	}
	return nil
}
