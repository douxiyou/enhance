//go:build !linux

package dhcp

import (
	"errors"
	"net"

	"github.com/insomniacslk/dhcp/dhcpv4"
)

func (h *handler4) sendEthernet(_ net.Interface, _ *dhcpv4.DHCPv4) error {
	return errors.New("当前平台不支持 sendEthernet")
}
