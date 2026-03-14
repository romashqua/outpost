package nat

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/pion/stun/v3"
)

// STUNServer is a lightweight STUN server that responds to binding requests
// with the client's reflexive transport address. It implements RFC 5389.
type STUNServer struct {
	listenAddr string
	logger     *slog.Logger
	conn       net.PacketConn
}

// NewSTUNServer creates a new STUN server listening on the given address.
func NewSTUNServer(listenAddr string, logger *slog.Logger) *STUNServer {
	return &STUNServer{
		listenAddr: listenAddr,
		logger:     logger,
	}
}

// Start begins serving STUN binding requests. It blocks until the context
// is cancelled or an unrecoverable error occurs.
func (s *STUNServer) Start(ctx context.Context) error {
	var err error
	s.conn, err = net.ListenPacket("udp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("stun listen %s: %w", s.listenAddr, err)
	}
	s.logger.Info("STUN server started", "addr", s.listenAddr)

	go func() {
		<-ctx.Done()
		s.conn.Close()
	}()

	buf := make([]byte, 1500)
	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("stun read: %w", err)
			}
		}

		go s.handlePacket(buf[:n], addr)
	}
}

func (s *STUNServer) handlePacket(data []byte, addr net.Addr) {
	msg := new(stun.Message)
	msg.Raw = data
	if err := msg.Decode(); err != nil {
		s.logger.Debug("invalid STUN message", "err", err, "from", addr.String())
		return
	}

	if msg.Type != stun.BindingRequest {
		s.logger.Debug("ignoring non-binding STUN message", "type", msg.Type, "from", addr.String())
		return
	}

	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		s.logger.Warn("unexpected addr type in STUN handler", "addr", addr.String())
		return
	}

	// Build response with XOR-MAPPED-ADDRESS.
	xorAddr := stun.XORMappedAddress{
		IP:   udpAddr.IP,
		Port: udpAddr.Port,
	}
	resp, err := stun.Build(stun.NewTransactionIDSetter(msg.TransactionID),
		stun.NewType(stun.MethodBinding, stun.ClassSuccessResponse),
		&xorAddr,
		stun.Fingerprint,
	)
	if err != nil {
		s.logger.Error("failed to build STUN response", "err", err)
		return
	}

	if _, err := s.conn.WriteTo(resp.Raw, addr); err != nil {
		s.logger.Error("failed to send STUN response", "err", err, "to", addr.String())
		return
	}

	s.logger.Debug("STUN binding response sent", "to", addr.String(), "mapped", xorAddr.String())
}

// Close shuts down the STUN server.
func (s *STUNServer) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
