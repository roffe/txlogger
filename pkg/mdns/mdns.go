package mdns

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv4"
)

var server *mdns.Conn

func init() {
	addr4, err := net.ResolveUDPAddr("udp4", mdns.DefaultAddressIPv4)
	if err != nil {
		log.Println("Failed to resolve mDNS address:", err)
		return
	}

	l4, err := net.ListenUDP("udp4", addr4)
	if err != nil {
		log.Println("Failed to listen on mDNS address:", err)
	}

	packetConnV4 := ipv4.NewPacketConn(l4)
	server, err = mdns.Server(packetConnV4, nil, &mdns.Config{})
	if err != nil {
		log.Println("Failed to create mDNS server:", err)
	}
}

// Query mDNS for a service
func Query(ctx context.Context, service string) (netip.Addr, error) {
	if server == nil {
		return netip.Addr{}, fmt.Errorf("mDNS server not initialized")
	}
	_, src, err := server.QueryAddr(ctx, service)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("failed to query mDNS: %w", err)
	}
	if src.IsValid() {
		log.Printf("Resolved mDNS address for service %s: %s", service, src.String())
		return src, nil
	}
	return netip.Addr{}, fmt.Errorf("no valid mDNS response for service %s", service)
}
