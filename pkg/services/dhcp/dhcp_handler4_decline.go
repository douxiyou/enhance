package dhcp

import (
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
)

func (s *Service) HandleDHCPDecline4(req *Request4) *dhcpv4.DHCPv4 {
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
		// 因为这是 DHCP decline，IP 被认为已经被占用
		// 我们只有在没有 lease 的情况下才会到这里，所以设备被认为是外部管理的
		// 创建一个带有 "invalid" 标识符的 leave，这样就不会被选择
		match.Identifier = "invalid"
		return nil
	}
	// 由于没有进一步的请求来确认这个 lease，直接使用 scope 的 TTL 保存它
	err := match.Put(req.Context, match.scope.TTL)
	if err != nil {
		req.log.Warn("保存 lease 失败", zap.Error(err))
	}
	return nil
}
