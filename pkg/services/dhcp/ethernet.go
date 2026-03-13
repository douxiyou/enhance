// Copyright 2018-present the CoreDHCP Authors. All rights reserved
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

//go:build linux

package dhcp

import (
	"fmt"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"go.uber.org/zap"
)

// Credit to CoreDHCP
// https://github.com/coredhcp/coredhcp/blob/master/server/sendEthernet.go

// 此函数向 resp.ClientHWAddr 中定义的硬件地址发送单播,
// 第3层目标地址仍然是广播地址;
// iface: 应发送 DHCP 消息的接口;
// resp: 应发送的 DHCPv4 结构体;
func (h *handler4) sendEthernet(iface net.Interface, resp *dhcpv4.DHCPv4) error {
	eth := layers.Ethernet{
		EthernetType: layers.EthernetTypeIPv4,
		SrcMAC:       iface.HardwareAddr,
		DstMAC:       resp.ClientHWAddr,
	}
	ip := layers.IPv4{
		Version:  4,
		TTL:      64,
		SrcIP:    resp.ServerIPAddr,
		DstIP:    resp.YourIPAddr,
		Protocol: layers.IPProtocolUDP,
		Flags:    layers.IPv4DontFragment,
	}
	udp := layers.UDP{
		SrcPort: dhcpv4.ServerPort,
		DstPort: dhcpv4.ClientPort,
	}

	err := udp.SetNetworkLayerForChecksum(&ip)
	if err != nil {
		return fmt.Errorf("发送以太网: 无法设置网络层: %v", err)
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	// Decode a packet
	packet := gopacket.NewPacket(resp.ToBytes(), layers.LayerTypeDHCPv4, gopacket.NoCopy)
	dhcpLayer := packet.Layer(layers.LayerTypeDHCPv4)
	dhcp, ok := dhcpLayer.(gopacket.SerializableLayer)
	if !ok {
		return fmt.Errorf("层 %s 不可序列化", dhcpLayer.LayerType().String())
	}
	err = gopacket.SerializeLayers(buf, opts, &eth, &ip, &udp, dhcp)
	if err != nil {
		return fmt.Errorf("无法序列化层: %v", err)
	}
	data := buf.Bytes()

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0)
	if err != nil {
		return fmt.Errorf("发送以太网: 无法打开套接字: %v", err)
	}
	defer func() {
		err = syscall.Close(fd)
		if err != nil {
			h.service.log.Error("发送以太网: 无法关闭套接字", zap.Error(err))
		}
	}()

	err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	if err != nil {
		h.service.log.Error("发送以太网: 无法为套接字设置选项", zap.Error(err))
	}

	var hwAddr [8]byte
	copy(hwAddr[0:6], resp.ClientHWAddr[0:6])
	ethAddr := syscall.SockaddrLinklayer{
		Protocol: 0,
		Ifindex:  iface.Index,
		Halen:    6,
		Addr:     hwAddr, // not used
	}
	err = syscall.Sendto(fd, data, 0, &ethAddr)
	if err != nil {
		return fmt.Errorf("无法通过套接字发送帧: %v", err)
	}
	return nil
}
