package nat

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"regexp"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

// TURNServer wraps pion/turn to provide a TURN relay server for clients
// behind symmetric NATs that cannot establish direct WireGuard connections.
type TURNServer struct {
	listenAddr string
	realm      string
	externalIP string
	logger     *slog.Logger
	server     *turn.Server
}

// NewTURNServer creates a new TURN server. externalIP is the public IP that
// clients will use to reach the relay.
func NewTURNServer(listenAddr, realm, externalIP string, logger *slog.Logger) *TURNServer {
	return &TURNServer{
		listenAddr: listenAddr,
		realm:      realm,
		externalIP: externalIP,
		logger:     logger,
	}
}

// Start begins serving TURN allocation and relay requests. It blocks until
// the context is cancelled.
func (t *TURNServer) Start(ctx context.Context) error {
	udpListener, err := net.ListenPacket("udp4", t.listenAddr)
	if err != nil {
		return fmt.Errorf("turn listen %s: %w", t.listenAddr, err)
	}

	ip := net.ParseIP(t.externalIP)
	if ip == nil {
		udpListener.Close()
		return fmt.Errorf("invalid external IP for TURN: %s", t.externalIP)
	}

	// Use a long-term credential mechanism with realm-based auth.
	// In production, credentials come from the database (device tokens).
	t.server, err = turn.NewServer(turn.ServerConfig{
		Realm: t.realm,
		AuthHandler: func(username, realm string, srcAddr net.Addr) ([]byte, bool) {
			// Return the key for the user. In production this would
			// look up the device's TURN credentials from the database.
			// For now, use a static derivation: HMAC(username, realm).
			key := turn.GenerateAuthKey(username, realm, username)
			return key, true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: ip,
					Address:      "0.0.0.0",
				},
			},
		},
		LoggerFactory: newSlogLoggerFactory(t.logger),
	})
	if err != nil {
		udpListener.Close()
		return fmt.Errorf("turn server: %w", err)
	}

	t.logger.Info("TURN server started", "addr", t.listenAddr, "realm", t.realm, "external_ip", t.externalIP)

	<-ctx.Done()
	return t.server.Close()
}

// Close shuts down the TURN server.
func (t *TURNServer) Close() error {
	if t.server != nil {
		return t.server.Close()
	}
	return nil
}

// slogLoggerFactory adapts slog to pion's logging.LoggerFactory interface.
type slogLoggerFactory struct {
	logger *slog.Logger
}

func newSlogLoggerFactory(logger *slog.Logger) *slogLoggerFactory {
	return &slogLoggerFactory{logger: logger}
}

func (f *slogLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	return &slogLeveledLogger{logger: f.logger.With("scope", scope)}
}

// slogLeveledLogger adapts slog to pion's logging.LeveledLogger interface.
type slogLeveledLogger struct {
	logger *slog.Logger
}

func (l *slogLeveledLogger) Trace(msg string)                          { l.logger.Debug(msg) }
func (l *slogLeveledLogger) Tracef(format string, args ...interface{}) { l.logger.Debug(fmt.Sprintf(format, args...)) }
func (l *slogLeveledLogger) Debug(msg string)                          { l.logger.Debug(msg) }
func (l *slogLeveledLogger) Debugf(format string, args ...interface{}) { l.logger.Debug(fmt.Sprintf(format, args...)) }
func (l *slogLeveledLogger) Info(msg string)                           { l.logger.Info(msg) }
func (l *slogLeveledLogger) Infof(format string, args ...interface{})  { l.logger.Info(fmt.Sprintf(format, args...)) }
func (l *slogLeveledLogger) Warn(msg string)                           { l.logger.Warn(msg) }
func (l *slogLeveledLogger) Warnf(format string, args ...interface{})  { l.logger.Warn(fmt.Sprintf(format, args...)) }
func (l *slogLeveledLogger) Error(msg string)                          { l.logger.Error(msg) }
func (l *slogLeveledLogger) Errorf(format string, args ...interface{}) { l.logger.Error(fmt.Sprintf(format, args...)) }

// Compile-time interface check.
var _ logging.LeveledLogger = (*slogLeveledLogger)(nil)

// turnLoggerFactory also needs to satisfy logging.LoggerFactory from pion.
// pion/turn v4 uses logging.LoggerFactory which has NewLogger(scope string) logging.LeveledLogger.
// Our adapter returns logging.LeveledLogger which is an alias. Verify at compile time:
var _ interface {
	NewLogger(scope string) logging.LeveledLogger
} = (*slogLoggerFactory)(nil)

// validRealmRe validates TURN realm strings.
var validRealmRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidateRealm checks if a TURN realm string is well-formed.
func ValidateRealm(realm string) bool {
	return len(realm) > 0 && len(realm) <= 255 && validRealmRe.MatchString(realm)
}
