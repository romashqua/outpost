package nat

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun/v3"
)

// NATType represents the detected NAT type for a device.
type NATType string

const (
	NATTypeFullCone       NATType = "full_cone"
	NATTypeRestrictedCone NATType = "restricted_cone"
	NATTypePortRestricted NATType = "port_restricted"
	NATTypeSymmetric      NATType = "symmetric"
	NATTypeOpen           NATType = "open"
	NATTypeUnknown        NATType = "unknown"
)

// DiscoveryResult holds the outcome of a NAT type detection.
type DiscoveryResult struct {
	NATType      NATType `json:"nat_type"`
	ExternalIP   string  `json:"external_ip"`
	ExternalPort int     `json:"external_port"`
}

// stunTimeout is the deadline for individual STUN transactions.
const stunTimeout = 5 * time.Second

// Discover performs NAT type detection against two STUN server addresses.
// It follows RFC 3489-style detection: sends binding requests from two local
// ports and compares the mapped addresses to classify the NAT behavior.
//
// serverAddr1 and serverAddr2 should be "host:port" strings for two distinct
// STUN endpoints (or the same server if change-request is supported).
func Discover(serverAddr1, serverAddr2 string) (*DiscoveryResult, error) {
	// Step 1: get mapped address from first server using first local port.
	mapped1, err := getMappedAddress(serverAddr1)
	if err != nil {
		return &DiscoveryResult{NATType: NATTypeUnknown}, fmt.Errorf("stun request 1: %w", err)
	}

	// Detect open/no NAT by comparing local address.
	localIP, err := getOutboundIP()
	if err == nil && mapped1.IP.String() == localIP.String() {
		return &DiscoveryResult{
			NATType:      NATTypeOpen,
			ExternalIP:   mapped1.IP.String(),
			ExternalPort: mapped1.Port,
		}, nil
	}

	// Step 2: get mapped address from second server using a new local port.
	mapped2, err := getMappedAddress(serverAddr2)
	if err != nil {
		// If second server unreachable, we know there's NAT but can't classify further.
		return &DiscoveryResult{
			NATType:      NATTypeUnknown,
			ExternalIP:   mapped1.IP.String(),
			ExternalPort: mapped1.Port,
		}, nil
	}

	// Step 3: compare the two mapped addresses.
	if mapped1.IP.String() != mapped2.IP.String() || mapped1.Port != mapped2.Port {
		// Different external mappings per destination → symmetric NAT.
		return &DiscoveryResult{
			NATType:      NATTypeSymmetric,
			ExternalIP:   mapped1.IP.String(),
			ExternalPort: mapped1.Port,
		}, nil
	}

	// Same external mapping — now determine cone type.
	// Step 4: attempt to receive from a different source port on server2.
	// If we can receive, it's full cone; otherwise restricted cone.
	//
	// Without a cooperative STUN server that supports CHANGE-REQUEST, we
	// approximate: send from the same local port to server2 and see if
	// the mapping is preserved. If external port stays the same as step 1,
	// classify as restricted cone (port-restricted is a refinement).
	mapped3, err := getMappedAddressFromPort(serverAddr2, mapped1.Port)
	if err != nil {
		return &DiscoveryResult{
			NATType:      NATTypeRestrictedCone,
			ExternalIP:   mapped1.IP.String(),
			ExternalPort: mapped1.Port,
		}, nil
	}

	if mapped3.Port == mapped1.Port {
		return &DiscoveryResult{
			NATType:      NATTypeFullCone,
			ExternalIP:   mapped1.IP.String(),
			ExternalPort: mapped1.Port,
		}, nil
	}

	return &DiscoveryResult{
		NATType:      NATTypePortRestricted,
		ExternalIP:   mapped1.IP.String(),
		ExternalPort: mapped1.Port,
	}, nil
}

// mappedAddr holds a STUN-discovered external address.
type mappedAddr struct {
	IP   net.IP
	Port int
}

// getMappedAddress sends a STUN binding request to the server and returns the
// XOR-MAPPED-ADDRESS from the response.
func getMappedAddress(serverAddr string) (*mappedAddr, error) {
	conn, err := net.DialTimeout("udp", serverAddr, stunTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", serverAddr, err)
	}
	defer conn.Close()

	c, err := stun.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("stun client: %w", err)
	}
	defer c.Close()

	var xorAddr stun.XORMappedAddress
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	errCh := make(chan error, 1)
	if err := c.Start(message, func(res stun.Event) {
		if res.Error != nil {
			errCh <- res.Error
			return
		}
		if err := xorAddr.GetFrom(res.Message); err != nil {
			// Try non-XOR mapped address as fallback.
			var mapped stun.MappedAddress
			if err2 := mapped.GetFrom(res.Message); err2 != nil {
				errCh <- fmt.Errorf("no mapped address in response: %w", err)
				return
			}
			xorAddr.IP = mapped.IP
			xorAddr.Port = mapped.Port
		}
		errCh <- nil
	}); err != nil {
		return nil, fmt.Errorf("stun start: %w", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case <-time.After(stunTimeout):
		return nil, fmt.Errorf("stun timeout for %s", serverAddr)
	}

	return &mappedAddr{IP: xorAddr.IP, Port: xorAddr.Port}, nil
}

// getMappedAddressFromPort sends a STUN request from a specific local port.
func getMappedAddressFromPort(serverAddr string, localPort int) (*mappedAddr, error) {
	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", localPort))
	if err != nil {
		return nil, err
	}
	raddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", laddr, raddr)
	if err != nil {
		return nil, fmt.Errorf("dial from port %d: %w", localPort, err)
	}
	defer conn.Close()

	c, err := stun.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("stun client: %w", err)
	}
	defer c.Close()

	var xorAddr stun.XORMappedAddress
	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

	errCh := make(chan error, 1)
	if err := c.Start(message, func(res stun.Event) {
		if res.Error != nil {
			errCh <- res.Error
			return
		}
		if err := xorAddr.GetFrom(res.Message); err != nil {
			var mapped stun.MappedAddress
			if err2 := mapped.GetFrom(res.Message); err2 != nil {
				errCh <- fmt.Errorf("no mapped address: %w", err)
				return
			}
			xorAddr.IP = mapped.IP
			xorAddr.Port = mapped.Port
		}
		errCh <- nil
	}); err != nil {
		return nil, fmt.Errorf("stun start: %w", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case <-time.After(stunTimeout):
		return nil, fmt.Errorf("stun timeout")
	}

	return &mappedAddr{IP: xorAddr.IP, Port: xorAddr.Port}, nil
}

// getOutboundIP determines the preferred outbound IP of the machine by
// dialing a well-known address (no actual packets are sent for UDP).
func getOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}
