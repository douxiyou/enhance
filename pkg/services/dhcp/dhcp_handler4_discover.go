package dhcp

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

func (s *Service) HandleDHCPDiscover4(req *Request4) *dhcpv4.DHCPv4 {
	req.log.Debug("处理 DHCPv4 Discover", zap.Any("req", req))
	match := s.FindLease(req)
	if match == nil {
		scope := s.findScopeForRequest(req)
		if scope == nil {
			req.log.Info("未找到 scope")
			return nil
		}
		req.log.Debug("为新 lease 找到 scope", zap.String("scope", scope.Name))
		match = scope.leaseFor(req)
		if match == nil {
			return nil
		}
		err := match.Put(req.Context, int64(viper.GetInt("dhcp.lease_negotiate_timeout")))
		if err != nil {
			req.log.Warn("在 discover 创建期间更新 lease 失败", zap.Error(err))
		}
	} else {
		err := match.Put(req.Context, match.scope.TTL)
		if err != nil {
			req.log.Warn("在 discover 期间更新 lease 失败", zap.Error(err))
		}
	}
	rep := match.createReply(req)
	rep.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	return rep
}
