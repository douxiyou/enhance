package dhcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"douxiyou.com/enhance/pkg/config"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
)

func getIP(addr net.Addr) net.IP {
	clientIP := ""
	switch addr := addr.(type) {
	case *net.UDPAddr:
		clientIP = addr.IP.String()
	}
	return net.ParseIP(clientIP)
}

var ErrNilResponse = errors.New("no DHCP response")

// 参考 CoreDHCP
// https://github.com/coredhcp/coredhcp/blob/master/server/handle.go

type handler4 struct {
	service *Service
	pc      *ipv4.PacketConn
	iface   net.Interface
}

// 性能方面，Pool 可能好也可能不好（参见 https://github.com/golang/go/issues/23199）
// Interface 对于我们想要的功能来说是好的。也许 "只是" 信任 GC，我们会没事的？
var bufpool = sync.Pool{New: func() interface{} { r := make([]byte, MaxDatagram); return &r }}

// MaxDatagram 是可以接收的消息的最大长度。
const MaxDatagram = 1 << 16

type Handler4 func(req *Request4) *dhcpv4.DHCPv4

func (h *handler4) Serve() error {
	for {
		b := *bufpool.Get().(*[]byte)
		b = b[:MaxDatagram] // 重新切片到最大容量，以防池中的缓冲区被切得更小

		n, oob, peer, err := h.pc.ReadFrom(b)
		if err != nil {
			return err
		}
		go func(buf []byte, oob *ipv4.ControlMessage, peer net.Addr) {
			_ = h.Handle(buf, oob, peer)
		}(b[:n], oob, peer.(*net.UDPAddr))
	}
}

var debugDHCPGatewayReplyPeer bool

func init() {
	debugDHCPGatewayReplyPeer = true
}

func (h *handler4) Handle(buf []byte, oob *ipv4.ControlMessage, peer net.Addr) error {
	if config.GetGlobalConfig().ListenOnlyMode {
		return nil
	}
	context, canc := context.WithCancel(h.service.ctx)
	defer canc()
	m, err := dhcpv4.FromBytes(buf)
	bufpool.Put(&buf)
	if err != nil {
		return fmt.Errorf("解析 dhcpv4 请求错误: %w", err)
	}

	r := h.service.NewRequest4(m)
	r.peer = peer
	r.Context = context
	r.oob = oob

	resp := h.HandleRequest(r)

	if resp == nil {
		r.log.Debug("handler4: 丢弃请求，因为响应为 nil")
		return ErrNilResponse
	}

	// h.service.logDHCPMessage(r, resp, []zapcore.Field{
	//  zap.String("type", "response"),
	// })
	useEthernet := false
	var p *net.UDPAddr
	if !r.GatewayIPAddr.IsUnspecified() {
		r.log.Debug("发送响应到网关")
		// giaddr 应该设置为中继的 IP 地址，但是它是客户端应该获取 IP 的子网的 IP
		// 我们可能并不总是能够直接回复那个 IP
		// 特别是在我们无法调整防火墙/路由规则的环境中（如 e2e 测试）
		// 当设置了这个环境变量时，直接回复我们收到 UDP 请求的 IP
		// 这不是 RFC 定义的行为
		p = &net.UDPAddr{IP: r.GatewayIPAddr, Port: dhcpv4.ServerPort}
		if debugDHCPGatewayReplyPeer {
			p.IP = getIP(r.peer)
		}
	} else if resp.MessageType() == dhcpv4.MessageTypeNak {
		r.log.Debug("发送响应到广播 (NAK)")
		p = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
	} else if !r.ClientIPAddr.IsUnspecified() {
		r.log.Debug("发送响应到客户端")
		p = &net.UDPAddr{IP: r.ClientIPAddr, Port: dhcpv4.ClientPort}
	} else if r.IsBroadcast() {
		r.log.Debug("发送响应到广播")
		p = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
	} else {
		// 发送 layer2 帧，以便我们可以定义目标 MAC 地址
		p = &net.UDPAddr{IP: resp.YourIPAddr, Port: dhcpv4.ClientPort}
		useEthernet = true
	}

	var woob *ipv4.ControlMessage
	if p.IP.Equal(net.IPv4bcast) || p.IP.IsLinkLocalUnicast() || useEthernet {
		// 直接广播、链路本地和 layer2 单播到接收请求的接口
		// 其他数据包应该使用正常的路由表
		// 以防不对称路由
		switch {
		case h.iface.Index != 0:
			woob = &ipv4.ControlMessage{IfIndex: h.iface.Index}
		case r.oob != nil && r.oob.IfIndex != 0:
			woob = &ipv4.ControlMessage{IfIndex: r.oob.IfIndex}
		default:
			r.log.Error("HandleMsg4: 未收到接口信息")
		}
	}

	if useEthernet {
		r.log.Debug("sending via ethernet (适配无IP/不规范客户端) 网卡索引:", zap.Int("ifIndex", woob.IfIndex))
		intf, err := net.InterfaceByIndex(woob.IfIndex)
		if err != nil {
			r.log.Error("handler4: 无法获取接口索引", zap.Error(err), zap.Int("index", woob.IfIndex))
			// 回退到 UDP 广播发送
			p = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
			useEthernet = false
		} else {
			err = h.sendEthernet(*intf, resp)
			if err != nil {
				r.log.Error("handler4: 无法发送以太网数据包，回退到 UDP 广播", zap.Error(err))
				// 回退到 UDP 广播发送
				p = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
				useEthernet = false
			}
		}
	}
	if !useEthernet {
		r.log.Debug("广播形式发送消息到设备")
		resp.SetBroadcast()
		p = &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpv4.ClientPort}
		r.log.Debug("广播形式发送消息到设备 具体数据是什么:", zap.String("resp", resp.String()))
		b := resp.ToBytes()
		n, err := h.pc.WriteTo(b, woob, p)
		if err != nil {
			r.log.Error("handler4: 连接写入失败", zap.Error(err), zap.String("peer", p.String()))
			return fmt.Errorf("handler4: 发送响应失败: %w", err)
		}
		if len(b) != n {
			r.log.Warn("handler4: 未发送所有字节", zap.Int("length", len(b)), zap.Int("sent", n))
		}
	}
	return nil
}

func (h *handler4) HandleRequest(r *Request4) *dhcpv4.DHCPv4 {
	if r.OpCode != dhcpv4.OpcodeBootRequest {
		h.service.log.Info("handler4: 不支持的操作码", zap.String("opcode", r.OpCode.String()))
		return nil
	}
	var handler Handler4
	switch mt := r.MessageType(); mt {
	case dhcpv4.MessageTypeDiscover:
		handler = h.service.HandleDHCPDiscover4
		h.service.log.Debug("handler4: 处理 discover 请求", zap.String("client", r.ClientIPAddr.String()))
	case dhcpv4.MessageTypeRequest:
		handler = h.service.HandleDHCPRequest4
		h.service.log.Debug("handler4: 处理 request 请求", zap.String("client", r.ClientIPAddr.String()))
	case dhcpv4.MessageTypeDecline:
		handler = h.service.HandleDHCPDecline4
		h.service.log.Debug("handler4: 处理 decline 请求", zap.String("client", r.ClientIPAddr.String()))
	case dhcpv4.MessageTypeRelease:
		handler = h.service.HandleDHCPRelease4
		h.service.log.Debug("handler4: 处理 release 请求", zap.String("client", r.ClientIPAddr.String()))
	default:
		r.log.Info("不支持的消息类型", zap.String("dhcpMsg", mt.String()))
		return nil
	}

	return h.service.recoverMiddleware4(
		h.service.loggingMiddleware4(
			handler,
		),
	)(r)
}
