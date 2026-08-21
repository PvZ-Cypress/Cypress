// Package cfb27mitm intercepts the game's traffic to EA and records it.
//
// The private server can only answer commands whose wire shape we know. Several
// Dynasty commands — notably the list/load pair behind the Load Dynasty menu —
// have never appeared in any capture, so they cannot be implemented from what we
// hold. This proxy exists to obtain that ground truth from a real session.
//
// The bridge rewrites the destination address of the game's connections to
// loopback while leaving the port intact, so a listener per observed port sees
// the original TLS stream. Every connection carries SNI, which names the real
// upstream host, so no side channel is needed to route: read the ClientHello,
// dial the named host on the same port, and relay between the two.
//
// Interception is possible at all because the bridge already forces the game's
// ProtoSSL certificate check to succeed, so the leaf presented here does not
// need to chain to EA's pinned CA.
package cfb27mitm

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cypress-servers/internal/blaze"
)

// Ports the game was observed dialing. 27920 carries Blaze; the rest are HTTPS
// services whose bodies are still worth recording.
var DefaultPorts = []int{27920, 443, 11000, 44325}

// PortOffset must match kCapturePortOffset in the bridge. Listening on shifted
// ports keeps the proxy clear of the private Blaze server, which owns 27920 —
// the same port EA's Blaze servers use — so a capture needs no teardown first.
const PortOffset = 10000

type Config struct {
	Bind      string
	Ports     []int
	LogFile   string
	BlazePort int
}

type Service struct {
	config    Config
	certMu    sync.Mutex
	certs     map[string]*tls.Certificate
	caCert    *x509.Certificate
	caKey     *ecdsa.PrivateKey
	logMu     sync.Mutex
	log       *os.File
	connSeq   uint64
	connSeqMu sync.Mutex
}

// Record is one line of the capture. Blaze frames carry the decoded fields;
// other traffic is recorded as bounded hex so nothing is silently dropped.
type Record struct {
	Time         time.Time     `json:"time"`
	ConnectionID string        `json:"connectionId"`
	Direction    string        `json:"direction"`
	ServerName   string        `json:"serverName,omitempty"`
	UpstreamPort int           `json:"upstreamPort,omitempty"`
	Kind         string        `json:"kind"`
	Component    uint16        `json:"component,omitempty"`
	Command      uint16        `json:"command,omitempty"`
	MessageType  uint8         `json:"messageType"`
	MessageID    uint32        `json:"messageId,omitempty"`
	ErrorCode    uint16        `json:"errorCode"`
	PayloadSize  int           `json:"payloadSize"`
	PayloadHex   string        `json:"payloadHex,omitempty"`
	Decoded      []blaze.Field `json:"decoded,omitempty"`
	DecodeError  string        `json:"decodeError,omitempty"`
	Note         string        `json:"note,omitempty"`
}

func NewService(cfg Config) (*Service, error) {
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if len(cfg.Ports) == 0 {
		cfg.Ports = DefaultPorts
	}
	if cfg.BlazePort == 0 {
		cfg.BlazePort = 27920
	}
	if cfg.LogFile == "" {
		cfg.LogFile = "cfb27-mitm.jsonl"
	}
	svc := &Service{config: cfg, certs: make(map[string]*tls.Certificate)}
	if err := svc.initCA(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o755); err != nil {
		return nil, fmt.Errorf("create capture directory: %w", err)
	}
	file, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open capture log: %w", err)
	}
	svc.log = file
	return svc, nil
}

func (s *Service) Close() error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if s.log == nil {
		return nil
	}
	err := s.log.Close()
	s.log = nil
	return err
}

func (s *Service) Run(ctx context.Context) error {
	var listeners []net.Listener
	for _, port := range s.config.Ports {
		address := net.JoinHostPort(s.config.Bind, strconv.Itoa(port+PortOffset))
		listener, err := net.Listen("tcp", address)
		if err != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			return fmt.Errorf("listen on %s: %w", address, err)
		}
		listeners = append(listeners, listener)
		fmt.Printf("CFB27 capture proxy listening on %s -> upstream port %d\n", address, port)
	}
	fmt.Printf("recording to %s\n", s.config.LogFile)

	go func() {
		<-ctx.Done()
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()

	var wg sync.WaitGroup
	for index, listener := range listeners {
		wg.Add(1)
		go func(listener net.Listener, port int) {
			defer wg.Done()
			for {
				client, err := listener.Accept()
				if err != nil {
					return
				}
				go s.serve(ctx, client, port)
			}
		}(listener, s.config.Ports[index])
	}
	wg.Wait()
	return nil
}

func (s *Service) nextConnectionID() string {
	s.connSeqMu.Lock()
	defer s.connSeqMu.Unlock()
	s.connSeq++
	return fmt.Sprintf("m-%06d", s.connSeq)
}

func (s *Service) serve(ctx context.Context, client net.Conn, port int) {
	connectionID := s.nextConnectionID()
	defer client.Close()

	// GetConfigForClient is the only place the SNI is available before the
	// handshake completes, and it is what names the upstream host.
	var serverName string
	tlsConfig := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			serverName = hello.ServerName
			if serverName == "" {
				return nil, errors.New("client sent no SNI; upstream host is unknown")
			}
			return s.certificateFor(serverName)
		},
		MinVersion: tls.VersionTLS12,
	}
	secure := tls.Server(client, tlsConfig)
	_ = secure.SetDeadline(time.Now().Add(30 * time.Second))
	if err := secure.HandshakeContext(ctx); err != nil {
		s.record(Record{
			Time: time.Now().UTC(), ConnectionID: connectionID, Direction: "client",
			ServerName: serverName, UpstreamPort: port, Kind: "handshake-failed",
			Note: err.Error(),
		})
		return
	}
	_ = secure.SetDeadline(time.Time{})

	upstreamAddress := net.JoinHostPort(serverName, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	upstream, err := tls.DialWithDialer(dialer, "tcp", upstreamAddress, &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		s.record(Record{
			Time: time.Now().UTC(), ConnectionID: connectionID, Direction: "upstream",
			ServerName: serverName, UpstreamPort: port, Kind: "dial-failed", Note: err.Error(),
		})
		return
	}
	defer upstream.Close()

	s.record(Record{
		Time: time.Now().UTC(), ConnectionID: connectionID, Direction: "upstream",
		ServerName: serverName, UpstreamPort: port, Kind: "established",
	})

	isBlaze := port == s.config.BlazePort
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.pipe(connectionID, "client_to_server", serverName, port, isBlaze, upstream, secure)
		// Half-close so the peer sees end-of-stream rather than hanging.
		_ = upstream.CloseWrite()
	}()
	go func() {
		defer wg.Done()
		s.pipe(connectionID, "server_to_client", serverName, port, isBlaze, secure, upstream)
		_ = secure.CloseWrite()
	}()
	wg.Wait()

	s.record(Record{
		Time: time.Now().UTC(), ConnectionID: connectionID, Direction: "upstream",
		ServerName: serverName, UpstreamPort: port, Kind: "closed",
	})
}

// pipe relays src into dst verbatim while feeding a copy to the recorder. The
// relay must never depend on parsing succeeding: a decode failure is logged and
// the bytes still go through untouched.
func (s *Service) pipe(
	connectionID, direction, serverName string,
	port int,
	isBlaze bool,
	dst io.Writer,
	src io.Reader,
) {
	var pending []byte
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if _, writeErr := dst.Write(chunk); writeErr != nil {
				return
			}
			if isBlaze {
				pending = append(pending, chunk...)
				pending = s.drainBlazeFrames(connectionID, direction, serverName, port, pending)
			} else {
				s.recordOpaque(connectionID, direction, serverName, port, chunk)
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (s *Service) drainBlazeFrames(
	connectionID, direction, serverName string,
	port int,
	pending []byte,
) []byte {
	for {
		reader := bytes.NewReader(pending)
		frame, err := blaze.ReadFrame(reader)
		if err != nil {
			// Incomplete or unparsable; keep buffering. Guard against a runaway
			// buffer if the stream is not actually Blaze.
			if len(pending) > 8<<20 {
				s.record(Record{
					Time: time.Now().UTC(), ConnectionID: connectionID, Direction: direction,
					ServerName: serverName, UpstreamPort: port, Kind: "unparsed-stream",
					PayloadSize: len(pending), Note: "buffer exceeded 8 MiB without a valid frame",
				})
				return nil
			}
			return pending
		}
		consumed := len(pending) - reader.Len()
		record := Record{
			Time: time.Now().UTC(), ConnectionID: connectionID, Direction: direction,
			ServerName: serverName, UpstreamPort: port, Kind: "blaze",
			Component: frame.Header.Component, Command: frame.Header.Command,
			MessageType: uint8(frame.Header.MessageType), MessageID: frame.Header.MessageID,
			ErrorCode: frame.Header.ErrorCode, PayloadSize: len(frame.Payload),
			PayloadHex: hex.EncodeToString(frame.Payload),
		}
		if decoded, decodeErr := blaze.Decode(frame.Payload); decodeErr != nil {
			record.DecodeError = decodeErr.Error()
		} else {
			record.Decoded = decoded
		}
		s.record(record)
		pending = pending[consumed:]
		if len(pending) == 0 {
			return nil
		}
	}
}

const opaquePreviewLimit = 4096

func (s *Service) recordOpaque(connectionID, direction, serverName string, port int, chunk []byte) {
	preview := chunk
	truncated := ""
	if len(preview) > opaquePreviewLimit {
		preview = preview[:opaquePreviewLimit]
		truncated = fmt.Sprintf("truncated from %d bytes", len(chunk))
	}
	s.record(Record{
		Time: time.Now().UTC(), ConnectionID: connectionID, Direction: direction,
		ServerName: serverName, UpstreamPort: port, Kind: "opaque",
		PayloadSize: len(chunk), PayloadHex: hex.EncodeToString(preview), Note: truncated,
	})
}

func (s *Service) record(record Record) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if s.log == nil {
		return
	}
	_ = json.NewEncoder(s.log).Encode(record)
}

func (s *Service) initCA() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Cypress CFB27 Capture CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	s.caCert = certificate
	s.caKey = key
	return nil
}

func (s *Service) certificateFor(serverName string) (*tls.Certificate, error) {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	if existing, ok := s.certs[serverName]; ok {
		return existing, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(serverName); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{serverName}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, s.caCert, &key.PublicKey, s.caKey)
	if err != nil {
		return nil, err
	}
	certificate := &tls.Certificate{
		Certificate: [][]byte{der, s.caCert.Raw},
		PrivateKey:  key,
	}
	s.certs[serverName] = certificate
	return certificate, nil
}

// RedactRecord strips credential-bearing values so a capture can be shared. The
// tokens the game exchanges are long-lived and tied to a real EA account, so a
// raw capture must never be committed.
func RedactRecord(record *Record) {
	const redacted = "<redacted>"
	for index := range record.Decoded {
		field := &record.Decoded[index]
		switch strings.ToUpper(field.Tag) {
		case "AUTH", "KEY", "SKEY", "TOKN", "TOKE", "PASS", "MAIL", "NAME", "DSNM", "PSID", "UID", "NID":
			if _, ok := field.Value.(string); ok {
				field.Value = redacted
			}
		}
	}
}
