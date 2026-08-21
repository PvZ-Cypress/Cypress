package cfb27blaze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27assets"
)

type Config struct {
	Bind            string
	Port            int
	DiagnosticsBind string
	DiagnosticsPort int
	LogFile         string
	RunID           string
	Profile         string
	DynastyURL      string
	CoachCatalog    string
	PersistDynasty  bool
	TLSMode         string
}

type Service struct {
	config               Config
	startedAt            time.Time
	handlers             map[route]handler
	rawHandlers          map[route][]byte
	rawRequestHandlers   map[route]rawRequestHandler
	dynastyNotifications map[route]map[uint32][]blaze.Frame
	dynasty              *DynastyClient
	coachCatalog         *cfb27assets.CoachCatalog
	coachCatalogError    error
	tlsConfig            *tls.Config
	tlsError             error
	eventsMu             sync.RWMutex
	events               []Event
	eventLogMu           sync.Mutex
	eventLog             *os.File
	connections          sync.Map
	acceptWG             sync.WaitGroup
	connectionWG         sync.WaitGroup
	connection           atomic.Uint64
	// Per-connection state (identity, selected team, notification counters) now
	// lives on clientSession, keyed by connection id, so multiple players do not
	// share one selected team / identity. See client_session.go.
	// frameWriters maps connectionID -> *connWriter. Unlike `connections` (which
	// holds the raw socket) this holds the writer frames actually go through —
	// the TLS wrapper once the handshake completes.
	frameWriters   sync.Map
	clientSessions sync.Map // connectionID -> *clientSession (many per player)
	playerSessions sync.Map // peer IP -> *clientSession (one per player)
	nextPlayerSlot atomic.Uint64
	fallback       *clientSession
	fallbackOnce   sync.Once
	// Seeded defaults a new connection inherits (the local dynasty seeded at
	// startup), so single-player keeps working exactly as before.
	// H2H game sessions (Blaze GameManager). Peer-to-peer brokering only.
	games         *gameRegistry
	writeMu       sync.Mutex
	seededSession atomic.Int64
	seededTeam    atomic.Int64
}

type route struct {
	component uint16
	command   uint16
}

type Event struct {
	Time          time.Time     `json:"time"`
	RunID         string        `json:"runId,omitempty"`
	ConnectionID  string        `json:"connectionId"`
	Status        string        `json:"status,omitempty"`
	RemoteAddr    string        `json:"remoteAddr,omitempty"`
	HTTPMethod    string        `json:"httpMethod,omitempty"`
	HTTPPath      string        `json:"httpPath,omitempty"`
	HTTPHost      string        `json:"httpHost,omitempty"`
	HTTPStatus    int           `json:"httpStatus,omitempty"`
	GRPCStatus    string        `json:"grpcStatus,omitempty"`
	BodyBytes     int           `json:"bodyBytes,omitempty"`
	TLSServerName string        `json:"tlsServerName,omitempty"`
	TLSVersions   []string      `json:"tlsVersions,omitempty"`
	TLSCiphers    []string      `json:"tlsCiphers,omitempty"`
	TLSCurves     []string      `json:"tlsCurves,omitempty"`
	TLSSignatures []string      `json:"tlsSignatures,omitempty"`
	TLSNextProtos []string      `json:"tlsNextProtos,omitempty"`
	MessageID     uint32        `json:"messageId"`
	Component     uint16        `json:"component"`
	Command       uint16        `json:"command"`
	MessageType   uint8         `json:"messageType"`
	ErrorCode     uint16        `json:"errorCode"`
	PayloadHex    string        `json:"payloadHex,omitempty"`
	Decoded       []blaze.Field `json:"decoded,omitempty"`
	DecodeError   string        `json:"decodeError,omitempty"`
	Transport     string        `json:"transport,omitempty"`
	DurationMS    float64       `json:"durationMs"`
}

func NewService(cfg Config) *Service {
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 27920
	}
	if cfg.DiagnosticsBind == "" {
		cfg.DiagnosticsBind = "127.0.0.1"
	}
	if cfg.DiagnosticsPort == 0 {
		cfg.DiagnosticsPort = 27921
	}
	if cfg.Profile == "" {
		cfg.Profile = "LocalPlayer"
	}
	if cfg.DynastyURL == "" {
		cfg.DynastyURL = "http://127.0.0.1:27910"
	}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "tls13"
	}
	rawHandlers := capturedSettingsPayloads()
	for route, payload := range capturedDynastyProgressionPayloads() {
		rawHandlers[route] = payload
	}
	for route, payload := range capturedStatsPayloads() {
		rawHandlers[route] = payload
	}
	svc := &Service{
		config:               cfg,
		startedAt:            time.Now().UTC(),
		handlers:             make(map[route]handler),
		rawHandlers:          rawHandlers,
		rawRequestHandlers:   make(map[route]rawRequestHandler),
		dynastyNotifications: capturedDynastyNotificationBatches(),
		dynasty:              NewDynastyClient(cfg.DynastyURL),
		events:               make([]Event, 0, 128),
		games:                newGameRegistry(),
	}
	if cfg.CoachCatalog != "" {
		svc.coachCatalog, svc.coachCatalogError = cfb27assets.LoadCoachCatalog(cfg.CoachCatalog)
	}
	svc.tlsConfig, svc.tlsError = newTLSConfig(cfg.TLSMode, net.ParseIP(cfg.Bind))
	svc.rawRequestHandlers = capturedDynastyFormHandlers(svc)
	svc.registerDefaultHandlers()
	svc.registerGameManagerHandlers()
	return svc
}

func (s *Service) Run(ctx context.Context) error {
	if s.coachCatalogError != nil {
		return fmt.Errorf("load authoritative coach catalog: %w", s.coachCatalogError)
	}
	if err := s.openEventLog(); err != nil {
		return err
	}
	defer s.closeEventLog()

	listeners, err := s.listenBlaze()
	if err != nil {
		return err
	}
	defer func() {
		closeListeners(listeners)
		s.acceptWG.Wait()
		s.closeActiveConnections()
		s.connectionWG.Wait()
	}()

	diagnostics := &http.Server{
		Addr:              net.JoinHostPort(s.config.DiagnosticsBind, strconv.Itoa(s.config.DiagnosticsPort)),
		Handler:           s.DiagnosticsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	diagnosticsErrors := make(chan error, 1)
	go func() {
		err := diagnostics.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			diagnosticsErrors <- err
		}
	}()
	defer diagnostics.Shutdown(context.Background())

	go func() {
		<-ctx.Done()
		closeListeners(listeners)
		s.closeActiveConnections()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = diagnostics.Shutdown(shutdownCtx)
	}()

	session, err := s.dynasty.EnsureSeeded(ctx, "Local Dynasty")
	if err != nil {
		return fmt.Errorf("seed local dynasty: %w", err)
	}
	s.seededSession.Store(session.ID)
	if session.SelectedTeamKey > 0 {
		s.seededTeam.Store(session.SelectedTeamKey)
	}
	fmt.Printf("CFB27 Blaze service listening on tcp://%s:%d (diagnostics http://%s:%d)\n",
		s.config.Bind, s.config.Port, s.config.DiagnosticsBind, s.config.DiagnosticsPort)

	acceptErrors := make(chan error, len(listeners))
	for _, listener := range listeners {
		s.acceptWG.Add(1)
		go func(listener net.Listener) {
			defer s.acceptWG.Done()
			for {
				connection, err := listener.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					acceptErrors <- err
					return
				}
				if tcp, ok := connection.(*net.TCPConn); ok {
					_ = tcp.SetKeepAlive(true)
					_ = tcp.SetKeepAlivePeriod(30 * time.Second)
					_ = tcp.SetNoDelay(true)
				}
				seq := s.connection.Add(1)
				id := fmt.Sprintf("c-%06d", seq)
				s.connectionWG.Add(1)
				go func() {
					defer s.connectionWG.Done()
					s.serveConnection(ctx, id, seq, connection)
				}()
			}
		}(listener)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case diagnosticsErr := <-diagnosticsErrors:
			return diagnosticsErr
		case err := <-acceptErrors:
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func (s *Service) listenBlaze() ([]net.Listener, error) {
	address := net.JoinHostPort(s.config.Bind, strconv.Itoa(s.config.Port))
	primary, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	listeners := []net.Listener{primary}
	loopback := s.config.Bind == "127.0.0.1" || s.config.Bind == "localhost"
	if loopback {
		if loopback6, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(s.config.Port))); err == nil {
			listeners = append(listeners, loopback6)
		}
	} else {
		// Hosting on a LAN/VPN address: also keep loopback open. Anything on this
		// machine that still dials 127.0.0.1 (and loopback is deliberately never
		// redirected by the bridge) would otherwise find nothing listening.
		if loopback4, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.config.Port))); err == nil {
			listeners = append(listeners, loopback4)
		}
		if loopback6, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(s.config.Port))); err == nil {
			listeners = append(listeners, loopback6)
		}
	}
	return listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func (s *Service) serveConnection(ctx context.Context, connectionID string, seq uint64, connection net.Conn) {
	s.connections.Store(connectionID, connection)
	// Give this connection its own identity + selected team + notification
	// counters, and carry it on the context so every handler for this connection
	// sees the same session. Connection 1 reproduces the historical identity.
	var peerIP uint32
	var peerPort int64
	if connection.RemoteAddr() != nil {
		peerIP, peerPort = ipv4ToUint32(connection.RemoteAddr())
	}
	session := s.sessionForPeer(connectionID, peerIP, peerPort)
	// Only the host (first connection) resumes the seeded team; a joining player
	// starts with no team so they pick their own rather than inheriting the
	// host's selection.
	ctx = withClientSession(ctx, session)
	ctx = withConnectionID(ctx, connectionID)
	defer func() {
		if session.releaseConnection() <= 0 {
			s.games.remove(session.playerKey)
		}
		s.dropClientSession(connectionID)
		s.connections.Delete(connectionID)
		_ = connection.Close()
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "closed",
			Transport:    "tcp",
		})
	}()
	remote := ""
	if connection.RemoteAddr() != nil {
		remote = connection.RemoteAddr().String()
	}
	s.record(Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "accepted",
		RemoteAddr:   remote,
		Transport:    "tcp",
	})
	reader := bufio.NewReader(connection)
	_ = connection.SetReadDeadline(time.Now().Add(15 * time.Second))
	first, err := reader.Peek(1)
	_ = connection.SetReadDeadline(time.Time{})
	if err != nil {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "peek-failed",
			RemoteAddr:   remote,
			Transport:    "tcp",
			DecodeError:  err.Error(),
		})
		return
	}
	s.recordFirstBytes(connectionID, remote, "tcp", reader, connection)

	transport := "plain"
	var frameReader io.Reader = reader
	var frameWriter io.Writer = connection
	if s.serveHTTPIfPresent(ctx, connectionID, remote, transport, reader, connection) {
		return
	}
	if first[0] == 0x16 {
		transport = "tls"
		// A certificate-pinned host cannot be impersonated, so hand the whole
		// connection to the real EA before terminating TLS.
		if serverName, sniErr := peekClientHelloSNI(reader); sniErr == nil && shouldTunnelSNI(serverName) {
			_ = connection.SetDeadline(time.Time{})
			event := Event{
				Time:          time.Now().UTC(),
				RunID:         s.config.RunID,
				ConnectionID:  connectionID,
				Status:        "tls-tunnel",
				RemoteAddr:    remote,
				TLSServerName: serverName,
				Transport:     transport,
			}
			s.record(event)
			sent, received, tunnelErr := tunnelToRealHost(serverName, reader, connection)
			event.Status = "tls-tunnel-closed"
			event.BodyBytes = int(received)
			event.DurationMS = float64(sent)
			if tunnelErr != nil {
				event.Status = "tls-tunnel-failed"
				event.DecodeError = tunnelErr.Error()
			}
			s.record(event)
			return
		}
		if s.tlsError != nil {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Transport:    transport,
				DecodeError:  "TLS configuration: " + s.tlsError.Error(),
			})
			return
		}
		tlsConfig := s.tlsConfig.Clone()
		tlsConfig.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			s.record(Event{
				Time:          time.Now().UTC(),
				RunID:         s.config.RunID,
				ConnectionID:  connectionID,
				Status:        "tls-client-hello",
				RemoteAddr:    remote,
				Transport:     transport,
				TLSServerName: hello.ServerName,
				TLSVersions:   formatTLSUint16s(hello.SupportedVersions),
				TLSCiphers:    formatTLSUint16s(hello.CipherSuites),
				TLSCurves:     formatTLSCurveIDs(hello.SupportedCurves),
				TLSSignatures: formatTLSSignatureSchemes(hello.SignatureSchemes),
				TLSNextProtos: hello.SupportedProtos,
			})
			return nil, nil
		}
		tap := &tlsRecordTap{}
		serverWriteLogged := atomic.Bool{}
		secure := tls.Server(&bufferedConnection{
			Conn:   connection,
			reader: reader,
			tap:    tap,
			onWrite: func(payload []byte) {
				if !serverWriteLogged.CompareAndSwap(false, true) {
					return
				}
				preview := payload
				if len(preview) > 32 {
					preview = preview[:32]
				}
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Status:       "tls-server-first-write",
					RemoteAddr:   remote,
					PayloadHex:   hex.EncodeToString(preview),
					Transport:    transport,
					BodyBytes:    len(payload),
				})
			},
		}, tlsConfig)
		_ = secure.SetDeadline(time.Now().Add(60 * time.Second))
		if err := secure.HandshakeContext(ctx); err != nil {
			decodeError := "TLS handshake: " + err.Error()
			// The game's ProtoSSL client sends a plaintext TLS alert before it drops the
			// connection. Capturing its description tells us why it aborts: 70/71 =
			// protocol_version (it refuses our TLS 1.2 downgrade), 40 = handshake_failure
			// (no acceptable cipher/curve), 42/46/48 = bad/unsupported/unknown-CA
			// certificate (cert pinning — the BearSSL end_chain path). Without this the
			// error is only a generic "forcibly closed by the remote host".
			if tap.haveAlert {
				decodeError += fmt.Sprintf(" (client TLS alert level=%d description=%d %s)",
					tap.alertLevel, tap.alertDesc, tlsAlertName(tap.alertDesc))
			}
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "tls-handshake-failed",
				RemoteAddr:   remote,
				Transport:    transport,
				DecodeError:  decodeError,
			})
			return
		}
		_ = secure.SetDeadline(time.Time{})
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "tls-handshake-ok",
			RemoteAddr:   remote,
			Transport:    transport,
		})
		// HTTP/2 is negotiated explicitly through TLS ALPN. Hand the raw TLS
		// connection to net/http before creating or peeking through a buffered
		// reader: bytes read into that buffer would otherwise be invisible to the
		// HTTP/2 server, including the mandatory client connection preface.
		if secure.ConnectionState().NegotiatedProtocol == "h2" {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "http2-negotiated",
				RemoteAddr:   remote,
				Transport:    transport,
			})
			s.serveHTTP2Connection(ctx, connectionID, remote, transport, secure)
			return
		}
		secureReader := bufio.NewReader(secure)
		// Log the first decrypted bytes so the exact Blaze redirector request framing
		// can be captured from the real game (diagnostic for wire-format matching).
		_ = secure.SetReadDeadline(time.Now().Add(3 * time.Second))
		if decrypted, _ := secureReader.Peek(64); len(decrypted) > 0 {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "tls-first-bytes",
				RemoteAddr:   remote,
				PayloadHex:   hex.EncodeToString(decrypted),
				Transport:    transport,
			})
		}
		_ = secure.SetReadDeadline(time.Time{})
		if s.serveHTTPIfPresent(ctx, connectionID, remote, transport, secureReader, secure) {
			return
		}
		frameReader = secureReader
		frameWriter = secure
	}

	// Publish the real writer only now that TLS (if any) is resolved, so
	// notifications pushed from other goroutines go through the same encryption
	// and locking as this loop's replies.
	writer := s.registerFrameWriter(connectionID, frameWriter)
	defer s.dropFrameWriter(connectionID)

	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetDeadline(deadline)
		}
		request, err := blaze.ReadFrame(frameReader)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Transport:    transport,
					DecodeError:  err.Error(),
				})
			}
			return
		}
		if request.Header.Component == ComponentAuthentication && request.Header.Command == CommandAuthenticationLogin {
			notification, notificationErr := localUserAddedNotification(session.identity)
			if notificationErr != nil {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Status:       "login-notification-encode-failed",
					DecodeError:  notificationErr.Error(),
				})
				return
			}
			if err := writer.write(notification); err != nil {
				return
			}
		}
		for _, response := range s.responseSequence(ctx, connectionID, request) {
			if err := writer.write(response); err != nil {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Transport:    transport,
					MessageID:    request.Header.MessageID,
					Component:    request.Header.Component,
					Command:      request.Header.Command,
					DecodeError:  err.Error(),
				})
				return
			}
		}
		if request.Header.Component == ComponentAuthentication && request.Header.Command == CommandAuthenticationLogin {
			notification, notificationErr := localUserSessionDataNotification(session.identity)
			if notificationErr != nil {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Status:       "session-data-notification-encode-failed",
					DecodeError:  notificationErr.Error(),
				})
				return
			}
			if err := writer.write(notification); err != nil {
				return
			}
		}
	}
}

func (s *Service) responseSequence(ctx context.Context, connectionID string, request blaze.Frame) []blaze.Frame {
	reply := s.HandleFrame(ctx, connectionID, request)
	cs := s.clientSessionFrom(ctx)
	// Only a connection that actually speaks Blaze is a player. Sessions are
	// created for every socket — health checks, registration, asset fetches — and
	// counting those put phantom entries in the friends list.
	cs.markPlayer()
	r := route{request.Header.Component, request.Header.Command}
	var occurrence uint32
	switch r {
	case route{ComponentBootStatus, 304}:
		occurrence = cs.dynastyContract.Add(1)
	case route{ComponentBootStatus, 541}:
		occurrence = cs.dynastyHub.Add(1)
	case route{ComponentBootStatus, 107}:
		occurrence = cs.dynastyAdvance.Add(1)
	default:
		return []blaze.Frame{reply}
	}

	notifications := s.dynastyNotifications[r][occurrence]
	if len(notifications) == 0 {
		return []blaze.Frame{reply}
	}
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: cs.identity.blazeID},
		{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return []blaze.Frame{reply}
	}
	localized := make([]blaze.Frame, len(notifications))
	leagueID := cs.activeDynastySession.Load()
	if leagueID == 0 {
		leagueID = 1
	}
	for i, notification := range notifications {
		payload, err := localizeDynastyNotificationPayload(notification.Payload, leagueID)
		if err != nil {
			return []blaze.Frame{reply}
		}
		localized[i] = notification
		localized[i].Header.MessageID = 0
		localized[i].Metadata = append([]byte(nil), metadata...)
		localized[i].Payload = s.localizeSelectedTeam(cs, payload)
	}

	if r.command == 541 {
		return append(localized, reply)
	}
	if r.command == 107 && occurrence == 1 && len(localized) >= 2 {
		sequence := make([]blaze.Frame, 0, len(localized)+1)
		sequence = append(sequence, localized[:2]...)
		sequence = append(sequence, reply)
		sequence = append(sequence, localized[2:]...)
		return sequence
	}
	return append([]blaze.Frame{reply}, localized...)
}

func (s *Service) recordFirstBytes(connectionID string, remote string, transport string, reader *bufio.Reader, connection net.Conn) {
	_ = connection.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	preview, err := reader.Peek(16)
	_ = connection.SetReadDeadline(time.Time{})
	if len(preview) == 0 {
		return
	}
	event := Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "first-bytes",
		RemoteAddr:   remote,
		PayloadHex:   hex.EncodeToString(preview),
		Transport:    transport,
	}
	if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
		event.DecodeError = err.Error()
	}
	s.record(event)
}

type bufferedConnection struct {
	net.Conn
	reader  *bufio.Reader
	tap     *tlsRecordTap
	onWrite func([]byte)
}

func (c *bufferedConnection) Read(buffer []byte) (int, error) {
	n, err := c.reader.Read(buffer)
	if n > 0 && c.tap != nil {
		c.tap.consume(buffer[:n])
	}
	return n, err
}

func (c *bufferedConnection) Write(buffer []byte) (int, error) {
	if len(buffer) > 0 && c.onWrite != nil {
		c.onWrite(buffer)
	}
	return c.Conn.Write(buffer)
}

// tlsRecordTap walks the client->server byte stream, parsing TLS record headers to
// capture the first plaintext alert record. Handshake-phase alerts (before the client's
// ChangeCipherSpec) are unencrypted, so this reveals exactly why the game's ProtoSSL
// client aborts our local handshake. It is diagnostic only and never alters the bytes.
type tlsRecordTap struct {
	buf        []byte
	haveAlert  bool
	alertLevel byte
	alertDesc  byte
}

func (t *tlsRecordTap) consume(p []byte) {
	if t.haveAlert {
		return
	}
	t.buf = append(t.buf, p...)
	for len(t.buf) >= 5 {
		recordType := t.buf[0]
		recordLen := int(t.buf[3])<<8 | int(t.buf[4])
		if recordLen < 0 || recordLen > 1<<14 {
			// Not a sane TLS record header (likely encrypted data we can't frame); stop.
			t.buf = nil
			return
		}
		if len(t.buf) < 5+recordLen {
			break
		}
		payload := t.buf[5 : 5+recordLen]
		if recordType == 0x15 && len(payload) >= 2 {
			t.alertLevel = payload[0]
			t.alertDesc = payload[1]
			t.haveAlert = true
			return
		}
		t.buf = t.buf[5+recordLen:]
	}
	if len(t.buf) > 1<<16 {
		t.buf = t.buf[len(t.buf)-(1<<16):]
	}
}

func tlsAlertName(description byte) string {
	switch description {
	case 40:
		return "handshake_failure"
	case 42:
		return "bad_certificate"
	case 43:
		return "unsupported_certificate"
	case 44:
		return "certificate_revoked"
	case 45:
		return "certificate_expired"
	case 46:
		return "certificate_unknown"
	case 47:
		return "illegal_parameter"
	case 48:
		return "unknown_ca"
	case 50:
		return "decode_error"
	case 51:
		return "decrypt_error"
	case 70:
		return "protocol_version"
	case 71:
		return "insufficient_security"
	case 80:
		return "internal_error"
	case 90:
		return "user_canceled"
	case 109:
		return "missing_extension"
	case 112:
		return "unrecognized_name"
	default:
		return "unknown"
	}
}

func formatTLSUint16s(values []uint16) []string {
	if len(values) == 0 {
		return nil
	}
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, fmt.Sprintf("0x%04x", value))
	}
	return formatted
}

func formatTLSCurveIDs(values []tls.CurveID) []string {
	if len(values) == 0 {
		return nil
	}
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, fmt.Sprintf("0x%04x", uint16(value)))
	}
	return formatted
}

func formatTLSSignatureSchemes(values []tls.SignatureScheme) []string {
	if len(values) == 0 {
		return nil
	}
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, fmt.Sprintf("0x%04x", uint16(value)))
	}
	return formatted
}

func (s *Service) serveHTTPIfPresent(
	ctx context.Context,
	connectionID string,
	remote string,
	transport string,
	reader *bufio.Reader,
	connection net.Conn,
) bool {
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	prefix, err := reader.Peek(4)
	_ = connection.SetReadDeadline(time.Time{})
	if err != nil || !looksLikeHTTPPrefix(prefix) {
		return false
	}
	if bytes.Equal(prefix, []byte("PRI ")) {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "http2-preface",
			RemoteAddr:   remote,
			Transport:    transport,
		})
		secure, ok := connection.(*tls.Conn)
		if !ok {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "http2-unsupported-transport",
				RemoteAddr:   remote,
				Transport:    transport,
			})
			return true
		}
		s.serveHTTP2Connection(ctx, connectionID, remote, transport, secure)
		return true
	}
	s.serveHTTPConnection(ctx, connectionID, remote, transport, reader, connection)
	return true
}

type oneShotListener struct {
	connection net.Conn
	accepted   atomic.Bool
	closed     chan struct{}
	closeOnce  sync.Once
}

func newOneShotListener(connection net.Conn) *oneShotListener {
	return &oneShotListener{connection: connection, closed: make(chan struct{})}
}

func (l *oneShotListener) Accept() (net.Conn, error) {
	if l.accepted.CompareAndSwap(false, true) {
		return l.connection, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *oneShotListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *oneShotListener) Addr() net.Addr {
	return l.connection.LocalAddr()
}

func (s *Service) serveHTTP2Connection(
	ctx context.Context,
	connectionID string,
	remote string,
	transport string,
	connection *tls.Conn,
) {
	listener := newOneShotListener(connection)
	connectionClosed := make(chan struct{})
	var connectionClosedOnce sync.Once
	server := &http.Server{
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			s.serveHTTP2Request(connectionID, remote, transport, writer, request)
		}),
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed || state == http.StateHijacked {
				connectionClosedOnce.Do(func() { close(connectionClosed) })
			}
		},
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
	case <-connectionClosed:
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "http2-serve-failed",
				RemoteAddr:   remote,
				Transport:    transport,
				DecodeError:  err.Error(),
			})
		}
	}

	_ = listener.Close()
	_ = server.Close()
}

func (s *Service) serveHTTP2Request(
	connectionID string,
	remote string,
	transport string,
	writer http.ResponseWriter,
	request *http.Request,
) {
	defer request.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(request.Body, 1<<20+1))
	payload := body
	if len(payload) > 4096 {
		payload = payload[:4096]
	}

	s.record(Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "http2-request",
		RemoteAddr:   remote,
		HTTPMethod:   request.Method,
		HTTPPath:     request.URL.RequestURI(),
		HTTPHost:     request.Host,
		HTTPStatus:   http.StatusOK,
		BodyBytes:    len(body),
		PayloadHex:   hex.EncodeToString(payload),
		Transport:    transport,
	})
	if readErr != nil {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "http2-body-read-failed",
			RemoteAddr:   remote,
			Transport:    transport,
			DecodeError:  readErr.Error(),
		})
		return
	}

	// Observation mode forwards gRPC to the real EA and records the reply, so the
	// protobuf shapes we cannot capture any other way can be read off the log.
	if proxyAllEnabled() && !strings.Contains(strings.ToLower(request.Host), "127.0.0.1") {
		// Subscriptions are relayed chunk by chunk: buffering them would hold the
		// stream until timeout and lose the very events we are trying to observe.
		if streamingGRPCMethods[strings.ToLower(request.URL.Path)] {
			streamErr := s.proxyGRPCStream(request.Host, request, body, writer, func(chunk []byte) {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Status:       "grpc-stream-chunk",
					RemoteAddr:   remote,
					HTTPPath:     request.URL.RequestURI(),
					HTTPHost:     request.Host,
					BodyBytes:    len(chunk),
					PayloadHex:   hex.EncodeToString(chunk),
					Transport:    transport,
				})
			})
			if streamErr != nil {
				s.record(Event{
					Time:         time.Now().UTC(),
					RunID:        s.config.RunID,
					ConnectionID: connectionID,
					Status:       "grpc-stream-failed",
					RemoteAddr:   remote,
					HTTPPath:     request.URL.RequestURI(),
					HTTPHost:     request.Host,
					DecodeError:  streamErr.Error(),
					Transport:    transport,
				})
			}
			return
		}
		result, proxyErr := s.proxyGRPC(request.Host, request, body)
		event := Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "grpc-passthrough",
			RemoteAddr:   remote,
			HTTPMethod:   request.Method,
			HTTPPath:     request.URL.RequestURI(),
			HTTPHost:     request.Host,
			Transport:    transport,
		}
		if proxyErr != nil {
			event.Status = "grpc-passthrough-failed"
			event.DecodeError = proxyErr.Error()
			s.record(event)
		} else {
			event.HTTPStatus = result.status
			event.GRPCStatus = result.trailer.Get("Grpc-Status")
			event.BodyBytes = len(result.body)
			event.PayloadHex = hex.EncodeToString(result.body)
			s.record(event)
			writeProxiedGRPC(writer, result)
			return
		}
	}

	grpcStatus := "12"
	grpcMessage := "offline method not implemented"
	responseBody := []byte(nil)
	path := strings.ToLower(request.URL.Path)
	if path == "/eadp.social.presence.v1.presenceservice/connecttopresencesession" {
		// Announce everyone on this server as online, then hold the stream open.
		// Without these events the friends screen shows players offline and offers
		// no actions against them, so nobody can be invited.
		header := writer.Header()
		header.Set("Content-Type", "application/grpc")
		header.Set("Trailer", "Grpc-Status, Grpc-Message")
		writer.WriteHeader(http.StatusOK)
		flusher, _ := writer.(http.Flusher)
		now := time.Now().UTC()
		for _, event := range presenceEventsFor(s.connectedPlayers(s.callerPersonaFor(remote)), now) {
			if _, err := writer.Write(event); err != nil {
				return
			}
		}
		if flusher != nil {
			flusher.Flush()
		}
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "presence-announced",
			RemoteAddr:   remote,
			HTTPPath:     request.URL.RequestURI(),
			HTTPHost:     request.Host,
			Transport:    transport,
		})
		select {
		case <-request.Context().Done():
		case <-time.After(grpcStreamHold):
		}
		header.Set("Grpc-Status", "0")
		return
	}
	if path == "/eadp.social.presence.v1.presenceservice/createpresencesession" {
		grpcStatus = "0"
		grpcMessage = ""
		responseBody = createPresenceSessionResponse(s.callerPersonaFor(remote))
	} else if streamingGRPCMethods[path] {
		// An idle subscription stays open rather than ending straight away; see
		// streamingGRPCMethods for why closing it immediately spins the client.
		header := writer.Header()
		header.Set("Content-Type", "application/grpc")
		header.Set("Trailer", "Grpc-Status, Grpc-Message")
		writer.WriteHeader(http.StatusOK)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-request.Context().Done():
		case <-time.After(grpcStreamHold):
		}
		header.Set("Grpc-Status", "0")
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "grpc-stream-closed",
			RemoteAddr:   remote,
			HTTPPath:     request.URL.RequestURI(),
			HTTPHost:     request.Host,
			Transport:    transport,
		})
		return
	}
	if path == "/eadp.playercard.v1.playercardservice/getplayercard" {
		// The overlay draws a card per player. An empty card names nobody, which
		// shows as "unexpected condition" and can take the client down entirely.
		grpcStatus = "0"
		grpcMessage = ""
		responseBody = s.cardForCaller(remote)
	} else if path == "/eadp.playercard.v1.playercardservice/batchgetplayercards" ||
		path == "/eadp.playercard.v1.playercardservice/batchgetfriendsplayercards" {
		grpcStatus = "0"
		grpcMessage = ""
		responseBody = s.cardsForRequest()
	} else if path == "/eadp.friends.v1.friends/listfriends" {
		// The roster of players on THIS server, so a player can pick who to invite.
		// EA's own answer lists the player's real friends, all offline, because
		// they are connected here rather than to EA.
		grpcStatus = "0"
		grpcMessage = ""
		responseBody = s.friendsListForRequest(s.callerPersonaFor(remote))
	} else if emptyOKGRPCMethods[path] {
		// Read-only queries that have nothing to return offline. UNIMPLEMENTED is
		// honest but the client treats it as a service failure — GetView produced
		// "Unable to retrieve your progression information from EA Servers" and
		// tore down the H2H session. An empty success is the accurate answer: the
		// query worked, there is simply no data.
		grpcStatus = "0"
		grpcMessage = ""
		responseBody = grpcFrame(nil)
	} else if strings.Contains(path, "health") && strings.HasSuffix(path, "/check") {
		grpcStatus = "0"
		grpcMessage = ""
		// gRPC frame containing HealthCheckResponse{status: SERVING}.
		responseBody = []byte{0, 0, 0, 0, 2, 0x08, 0x01}
	} else if path == "/eadp.nexus.connect.grpc.v1.tokenservice/granttokenbyauthorizationcode" {
		grpcStatus = "0"
		grpcMessage = ""
		// The production wire response contains five nested values. Supplying only
		// AccessToken makes the SDK fall back to GetTokenInfo; supplying the local
		// IdToken lets it obtain the identity claims and proceed directly to Blaze.
		accessToken := protobufStringField(1, offlineAccessToken(time.Now().UTC()))
		tokenType := protobufStringField(1, "Bearer")
		accessExpiresIn := protobufVarintField(1, 259199)
		idToken := protobufStringField(1, offlineIDToken(time.Now().UTC()))
		idTokenExpiresIn := protobufVarintField(1, 777599)
		grantToken := protobufBytesField(1, accessToken)
		grantToken = append(grantToken, protobufBytesField(2, tokenType)...)
		grantToken = append(grantToken, protobufBytesField(3, accessExpiresIn)...)
		grantToken = append(grantToken, protobufBytesField(4, idToken)...)
		grantToken = append(grantToken, protobufBytesField(5, idTokenExpiresIn)...)
		responseBody = grpcFrame(grantToken)
	} else if path == "/eadp.nexus.connect.grpc.v1.tokeninfoservice/gettokeninfo" {
		grpcStatus = "0"
		grpcMessage = ""
		// GetTokenInfoResponse.token_info. These are the stable local identity
		// fields consumed before the SDK submits its Blaze login request.
		tokenInfo := protobufStringField(1, "CFB_27_PC_CLIENT")
		tokenInfo = append(tokenInfo, protobufStringField(2, "basic.identity basic.persona")...)
		tokenInfo = append(tokenInfo, protobufStringField(3, "pc")...)
		tokenInfo = append(tokenInfo, protobufStringField(4, "cypress-local-device")...)
		tokenInfo = append(tokenInfo, protobufStringField(5, "cem_ea_id")...)
		tokenInfo = append(tokenInfo, protobufStringField(6, "prod")...)
		tokenInfo = append(tokenInfo, protobufStringField(7, "cfb-2027-pc")...)
		tokenInfo = append(tokenInfo, protobufStringField(9, "1001")...)
		tokenInfo = append(tokenInfo, protobufStringField(13, "1001")...)
		responseBody = grpcFrame(protobufBytesField(1, tokenInfo))
	}

	header := writer.Header()
	header.Set("Content-Type", "application/grpc")
	header.Set("Trailer", "Grpc-Status, Grpc-Message")
	writer.WriteHeader(http.StatusOK)
	if len(responseBody) > 0 {
		_, _ = writer.Write(responseBody)
	}
	header.Set("Grpc-Status", grpcStatus)
	if grpcMessage != "" {
		header.Set("Grpc-Message", grpcMessage)
	}

	s.record(Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "http2-response",
		RemoteAddr:   remote,
		HTTPMethod:   request.Method,
		HTTPPath:     request.URL.RequestURI(),
		HTTPHost:     request.Host,
		HTTPStatus:   http.StatusOK,
		GRPCStatus:   grpcStatus,
		BodyBytes:    len(responseBody),
		PayloadHex:   hex.EncodeToString(responseBody),
		Transport:    transport,
	})
}

func grpcFrame(message []byte) []byte {
	frame := make([]byte, 5+len(message))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(message)))
	copy(frame[5:], message)
	return frame
}

func protobufStringField(fieldNumber uint64, value string) []byte {
	return protobufBytesField(fieldNumber, []byte(value))
}

func protobufVarintField(fieldNumber uint64, value uint64) []byte {
	result := appendProtobufVarint(nil, fieldNumber<<3)
	return appendProtobufVarint(result, value)
}

func protobufBytesField(fieldNumber uint64, value []byte) []byte {
	result := appendProtobufVarint(nil, fieldNumber<<3|2)
	result = appendProtobufVarint(result, uint64(len(value)))
	return append(result, value...)
}

func appendProtobufVarint(buffer []byte, value uint64) []byte {
	for value >= 0x80 {
		buffer = append(buffer, byte(value)|0x80)
		value >>= 7
	}
	return append(buffer, byte(value))
}

func offlineAccessToken(now time.Time) string {
	return offlineJWT(map[string]any{
		"iss": "accounts.ea.com",
		"jti": "cypress-offline-access-token",
		"azp": "CFB_27_PC_CLIENT",
		"iat": now.Unix(),
		"exp": now.Add(72 * time.Hour).Unix(),
		"ver": 1,
		"nexus": map[string]any{
			"rsvd":  map[string]any{"efplty": "11"},
			"cli":   "CFB_27_PC_CLIENT",
			"prd":   "r1eu",
			"sco":   "offline dp.client.default signin",
			"pid":   "1001",
			"pty":   "NUCLEUS",
			"uid":   "1001",
			"psid":  1001,
			"dvid":  "cypress-local-device",
			"pltyp": "PC",
			"pnid":  "EA",
			"dpid":  "PC",
			"stps":  "OFF",
			"udg":   false,
			"cnty":  "1",
			"ausrc": "325760",
			"psif": []map[string]any{{
				"id":  1001,
				"ns":  "cem_ea_id",
				"dis": "LocalPlayer",
				"nic": "LocalPlayer",
			}},
			"uif": map[string]any{
				"lan": "en",
				"sta": "ACTIVE",
				"agp": "ADULT",
				"age": 18,
				"cty": "US",
				"udg": false,
				"ano": false,
				"agv": false,
			},
			"enc": "cypress-local",
		},
	})
}

func offlineIDToken(now time.Time) string {
	return offlineJWT(map[string]any{
		"iss": "accounts.ea.com",
		"jti": "cypress-offline-id-token",
		"azp": "CFB_27_PC_CLIENT",
		"iat": now.Unix(),
		"exp": now.Add(216 * time.Hour).Unix(),
		"ver": 1,
		"nexus": map[string]any{
			"uid": "1001",
			"enc": "cypress-local",
		},
	})
}

func offlineJWT(claims map[string]any) string {
	header, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"kid": "cypress-local",
		"typ": "JWT",
	})
	payload, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("cypress-local-signature"))
}

func looksLikeHTTPPrefix(prefix []byte) bool {
	if len(prefix) < 4 {
		return false
	}
	switch string(prefix) {
	case "GET ", "POST", "HEAD", "PUT ", "DELE", "OPTI", "PATC", "CONN", "PRI ":
		return true
	default:
		return false
	}
}

func (s *Service) serveHTTPConnection(
	_ context.Context,
	connectionID string,
	remote string,
	transport string,
	reader *bufio.Reader,
	connection net.Conn,
) {
	request, err := http.ReadRequest(reader)
	if err != nil {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "http-read-failed",
			RemoteAddr:   remote,
			Transport:    transport,
			DecodeError:  err.Error(),
		})
		return
	}
	defer request.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(request.Body, 1<<20+1))
	payload := body
	if len(payload) > 64<<10 {
		payload = payload[:64<<10]
	}

	// Observation mode: forward everything upstream and record what was asked,
	// so the sequence the trial build needs can be read off a working session.
	if proxyAllEnabled() {
		if upstream := upstreamHostFromSNI(connection); upstream != "" {
			status, header, upstreamBody, proxyErr := s.proxyToHost(upstream, request, body)
			event := Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "proxy-all",
				RemoteAddr:   remote,
				HTTPMethod:   request.Method,
				HTTPPath:     request.URL.RequestURI(),
				HTTPHost:     upstream,
				HTTPStatus:   status,
				BodyBytes:    len(upstreamBody),
				Transport:    transport,
			}
			if proxyErr != nil {
				event.Status = "proxy-all-failed"
				event.DecodeError = proxyErr.Error()
				s.record(event)
				fmt.Fprint(connection, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return
			}
			fmt.Printf("proxy-all %s %s%s -> %d (%d bytes)\n",
				request.Method, upstream, request.URL.RequestURI(), status, len(upstreamBody))
			s.record(event)
			_ = writeProxiedResponse(connection, status, header, upstreamBody)
			return
		}
	}

	// Players announce who they are from their own machine, where the Cypress
	// launcher has already signed them in. That is more accurate than any list
	// kept on the host, and it is what makes a player addressable in an invite.
	if strings.HasPrefix(strings.ToLower(request.URL.Path), "/cypress/register") {
		name := request.URL.Query().Get("name")
		address := hostname(remote)
		s.registerPlayerName(address, name)
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "player-registered",
			RemoteAddr:   remote,
			HTTPPath:     request.URL.RequestURI(),
			Transport:    transport,
		})
		// Measure the body rather than predicting its length: a Content-Length
		// that overstates it leaves the client waiting for bytes that never
		// arrive, which surfaces as "the connection was closed".
		responseBody := fmt.Sprintf("{\"registered\":%q}", name)
		fmt.Fprintf(connection,
			"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
			len(responseBody), responseBody)
		return
	}

	// Static content (the roster database and other CDN assets) is fetched from
	// the real host. Standing in for it broke the roster download outright.
	if isAssetPassThroughHost(request.Host) {
		upstreamStatus, header, upstreamBody, proxyErr := s.proxyToHost(hostname(request.Host), request, body)
		event := Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "asset-passthrough",
			RemoteAddr:   remote,
			HTTPMethod:   request.Method,
			HTTPPath:     request.URL.RequestURI(),
			HTTPHost:     request.Host,
			HTTPStatus:   upstreamStatus,
			BodyBytes:    len(upstreamBody),
			Transport:    transport,
		}
		if proxyErr != nil {
			event.Status = "asset-passthrough-failed"
			event.DecodeError = proxyErr.Error()
			s.record(event)
			fmt.Fprint(connection, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			return
		}
		s.record(event)
		_ = writeProxiedResponse(connection, upstreamStatus, header, upstreamBody)
		return
	}

	// Sign-in for the trial build's HTTP OAuth flow is answered locally, so its
	// token matches the identity the rest of this server presents.
	if !authProxyEnabled() {
		if status, header, responseBody, handled := localAuthResponse(request); handled {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "auth-local",
				RemoteAddr:   remote,
				HTTPMethod:   request.Method,
				HTTPPath:     request.URL.RequestURI(),
				HTTPHost:     request.Host,
				HTTPStatus:   status,
				BodyBytes:    len(responseBody),
				Transport:    transport,
			})
			_ = writeProxiedResponse(connection, status, header, responseBody)
			return
		}
	}

	// Authentication is proxied to the real EA rather than answered locally.
	if isAuthPassThroughRequest(request.Host, request.URL.Path) {
		upstreamStatus, header, upstreamBody, proxyErr := s.proxyToRealEA(request, body)
		event := Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "auth-passthrough",
			RemoteAddr:   remote,
			HTTPMethod:   request.Method,
			HTTPPath:     request.URL.RequestURI(),
			HTTPHost:     request.Host,
			HTTPStatus:   upstreamStatus,
			BodyBytes:    len(upstreamBody),
			Transport:    transport,
		}
		if proxyErr != nil {
			event.Status = "auth-passthrough-failed"
			event.DecodeError = proxyErr.Error()
			s.record(event)
			fmt.Fprint(connection, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			return
		}
		s.record(event)
		_ = writeProxiedResponse(connection, upstreamStatus, header, upstreamBody)
		return
	}

	status := http.StatusOK
	contentType, responseBody := s.localHTTPResponse(request, body)
	s.record(Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "http-request",
		RemoteAddr:   remote,
		HTTPMethod:   request.Method,
		HTTPPath:     request.URL.RequestURI(),
		HTTPHost:     request.Host,
		HTTPStatus:   status,
		BodyBytes:    len(body),
		PayloadHex:   hex.EncodeToString(payload),
		Transport:    transport,
	})
	if readErr != nil {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "http-body-read-failed",
			RemoteAddr:   remote,
			Transport:    transport,
			DecodeError:  readErr.Error(),
		})
		return
	}

	_, err = fmt.Fprintf(
		connection,
		"HTTP/1.1 %d %s\r\nContent-Type: %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status,
		http.StatusText(status),
		contentType,
		len(responseBody),
		responseBody)
	if err != nil {
		s.record(Event{
			Time:         time.Now().UTC(),
			RunID:        s.config.RunID,
			ConnectionID: connectionID,
			Status:       "http-write-failed",
			RemoteAddr:   remote,
			Transport:    transport,
			DecodeError:  err.Error(),
		})
		return
	}
	responsePreview := responseBody
	if len(responsePreview) > 1024 {
		responsePreview = responsePreview[:1024]
	}
	s.record(Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "http-response",
		RemoteAddr:   remote,
		HTTPMethod:   request.Method,
		HTTPPath:     request.URL.RequestURI(),
		HTTPHost:     request.Host,
		HTTPStatus:   status,
		BodyBytes:    len(responseBody),
		PayloadHex:   hex.EncodeToString(responsePreview),
		Transport:    transport,
	})
}

const localBlazeIPv4 = uint32(2130706433) // 127.0.0.1 in network byte order.

// advertisedBlaze is the address the redirector hands the client as "where the
// Blaze server lives". It must be the address the server actually listens on:
// when hosting for a second player the server binds to a LAN/VPN address, and
// advertising 127.0.0.1 would send the client to its OWN loopback, where nothing
// is listening. The bridge cannot rescue that either, since loopback is
// deliberately never redirected — the symptom is "lost connection to the EA
// servers" with no Blaze frames reaching the server at all.
func (s *Service) advertisedBlaze() (string, uint32, int) {
	port := s.config.Port
	if port == 0 {
		port = 27920
	}
	host := s.config.Bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	v4 := net.IP(nil)
	if ip != nil {
		v4 = ip.To4()
	}
	if v4 == nil {
		return "127.0.0.1", localBlazeIPv4, port
	}
	packed := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	return host, packed, port
}

func (s *Service) localHTTPResponse(request *http.Request, requestBody []byte) (string, []byte) {
	if request.URL.Path != "/redirector/getServerInstance" {
		return "application/json", []byte("{}")
	}
	host, packed, port := s.advertisedBlaze()

	wantsXML := strings.Contains(strings.ToLower(request.Header.Get("Content-Type")), "xml") ||
		bytes.HasPrefix(bytes.TrimSpace(requestBody), []byte("<"))
	if wantsXML {
		return "application/xml; charset=utf-8", []byte(fmt.Sprintf(
			`<?xml version="1.0" encoding="UTF-8"?>
<serverinstanceresponse>
  <address>
    <ipaddress>
      <hostname>%s</hostname>
      <ip>%d</ip>
      <port>%d</port>
    </ipaddress>
  </address>
  <addressremaps></addressremaps>
  <certificatelist></certificatelist>
  <messages></messages>
  <nameremaps></nameremaps>
  <secure>true</secure>
  <trialservicename></trialservicename>
  <defaultdnsaddress>0</defaultdnsaddress>
</serverinstanceresponse>`,
			host, packed, port))
	}

	return "application/json", []byte(fmt.Sprintf(
		`{"address":{"ipAddress":{"hostname":"%s","ip":%d,"port":%d}},"addressRemaps":[],"certificateList":[],"messages":[],"nameRemaps":[],"secure":true,"trialServiceName":"","defaultDnsAddress":0}`,
		host, packed, port))
}

func (s *Service) HandleFrame(ctx context.Context, connectionID string, request blaze.Frame) blaze.Frame {
	started := time.Now()
	response := blaze.Frame{Header: blaze.Header{
		Component: request.Header.Component,
		Command:   request.Header.Command,
		UserIndex: request.Header.UserIndex,
		MessageID: request.Header.MessageID,
	}}

	cs := s.clientSessionFrom(ctx)
	if request.Header.MessageType == blaze.MessageTypePing {
		response.Header.MessageType = blaze.MessageTypePingReply
		response.Payload = append([]byte(nil), request.Payload...)
	} else if payload, ok := s.rawHandlers[route{request.Header.Component, request.Header.Command}]; ok {
		response.Header.MessageType = blaze.MessageTypeReply
		if request.Header.Component == ComponentBootStatus && request.Header.Command == 391 {
			// This is a league-wide team database. Rewriting the captured team's
			// identity here duplicates the selected team and removes Akron.
			// Team selection is an association, not replacement of a league row.
			response.Payload = append([]byte(nil), payload...)
		} else if request.Header.Component == ComponentBootStatus && request.Header.Command == 541 {
			response.Payload = s.localizeDynastyHub(cs, payload)
		} else if request.Header.Component == ComponentBootStatus && request.Header.Command == 393 {
			response.Payload = s.selectedDynastyRosterPayload(cs, payload)
		} else {
			response.Payload = s.localizeSelectedTeam(cs, payload)
		}
	} else if h, ok := s.rawRequestHandlers[route{request.Header.Component, request.Header.Command}]; ok {
		payload, errorCode := h(ctx, request)
		response.Header.ErrorCode = errorCode
		if errorCode == 0 {
			response.Header.MessageType = blaze.MessageTypeReply
			response.Payload = append([]byte(nil), payload...)
		} else {
			response.Header.MessageType = blaze.MessageTypeErrorReply
		}
	} else if h, ok := s.handlers[route{request.Header.Component, request.Header.Command}]; ok {
		fields, errorCode := h(ctx, request)
		response.Header.ErrorCode = errorCode
		if errorCode == 0 {
			response.Header.MessageType = blaze.MessageTypeReply
		} else {
			response.Header.MessageType = blaze.MessageTypeErrorReply
		}
		payload, err := blaze.Encode(fields)
		if err != nil {
			response.Header.MessageType = blaze.MessageTypeErrorReply
			response.Header.ErrorCode = ErrorSystem
		} else {
			response.Payload = payload
		}
		if errorCode == 0 && request.Header.Component == ComponentAuthentication && request.Header.Command == CommandAuthenticationLogin {
			metadata, metadataErr := blaze.Encode([]blaze.Field{
				{Tag: "CNTX", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "SKEY", Type: blaze.TypeString, Value: cs.identity.sessionKey},
			})
			if metadataErr != nil {
				response.Header.MessageType = blaze.MessageTypeErrorReply
				response.Header.ErrorCode = ErrorSystem
				response.Payload = nil
			} else {
				response.Metadata = metadata
			}
		}
	} else {
		response.Header.MessageType = blaze.MessageTypeErrorReply
		response.Header.ErrorCode = ErrorCommandNotFound
	}

	decoded, decodeErr := blaze.Decode(request.Payload)
	event := Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		MessageID:    request.Header.MessageID,
		Component:    request.Header.Component,
		Command:      request.Header.Command,
		MessageType:  uint8(request.Header.MessageType),
		ErrorCode:    response.Header.ErrorCode,
		PayloadHex:   hex.EncodeToString(request.Payload),
		Decoded:      decoded,
		DurationMS:   float64(time.Since(started).Microseconds()) / 1000,
	}
	if decodeErr != nil {
		event.DecodeError = decodeErr.Error()
	}
	s.record(event)
	return response
}

func (s *Service) Events() []Event {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()
	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

func (s *Service) record(event Event) {
	s.eventsMu.Lock()
	s.events = append(s.events, event)
	if len(s.events) > 500 {
		s.events = append([]Event(nil), s.events[len(s.events)-500:]...)
	}
	s.eventsMu.Unlock()

	s.eventLogMu.Lock()
	defer s.eventLogMu.Unlock()
	if s.eventLog != nil {
		_ = json.NewEncoder(s.eventLog).Encode(event)
	}
}

func (s *Service) openEventLog() error {
	if s.config.LogFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.config.LogFile), 0755); err != nil {
		return fmt.Errorf("create Blaze log directory: %w", err)
	}
	file, err := os.OpenFile(s.config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open Blaze event log: %w", err)
	}
	s.eventLogMu.Lock()
	s.eventLog = file
	s.eventLogMu.Unlock()
	return nil
}

func (s *Service) closeEventLog() {
	s.eventLogMu.Lock()
	defer s.eventLogMu.Unlock()
	if s.eventLog != nil {
		_ = s.eventLog.Close()
		s.eventLog = nil
	}
}

func (s *Service) closeActiveConnections() {
	s.connections.Range(func(_, value any) bool {
		if connection, ok := value.(net.Conn); ok {
			_ = connection.Close()
		}
		return true
	})
}

func (s *Service) activeConnectionCount() int {
	count := 0
	s.connections.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (s *Service) DiagnosticsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		health := map[string]any{
			"status":             "ok",
			"service":            "cfb27-blaze",
			"startedAt":          s.startedAt,
			"events":             len(s.Events()),
			"connections":        s.activeConnectionCount(),
			"tlsMode":            s.config.TLSMode,
			"runId":              s.config.RunID,
			"logFile":            s.config.LogFile,
			"coachCatalogStatus": "not-configured",
			"coachCount":         0,
			"teamCount":          0,
		}
		if s.coachCatalogError != nil {
			health["coachCatalogStatus"] = "error"
			health["coachCatalogError"] = s.coachCatalogError.Error()
		} else if s.coachCatalog != nil {
			health["coachCatalogStatus"] = "loaded"
			health["coachCatalogPath"] = s.config.CoachCatalog
			health["coachCount"] = len(s.coachCatalog.Coaches)
			health["teamCount"] = len(s.coachCatalog.Teams)
		}
		writeJSON(w, http.StatusOK, health)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"events": s.Events()})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
