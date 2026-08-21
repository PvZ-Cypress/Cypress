package cfb27blaze

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

/*
Pass a pinned TLS connection straight through to the real EA, unterminated.

The game's ProtoSSL pins EA's certificate authority when it dials the Blaze
redirector BY HOSTNAME (gosca25.blazeredirector.ea.com). Our self-signed
certificate is rejected: we send ServerHello, the client goes silent, and the
connection dies after ~30s. That is fatal for the trial build, which has no
other route to Blaze. (The full build survives it only because it falls back to
dialling Blaze by IP, and ProtoSSL does not pin on that path — which is why the
same bug looked like it only affected one machine.)

Forging a certificate the pinned CA accepts is not possible, so this connection
is not terminated at all: the ClientHello is parsed just far enough to read the
SNI, then the raw bytes are piped to the real host. The game completes its
handshake against EA's genuine certificate, pinning is satisfied, and the Blaze
address it is handed afterwards is dialled by IP — which the bridge redirects
here, unpinned, where we serve it normally.
*/

// tunnelHostSuffixes are dialled by hostname and certificate-pinned.
var tunnelHostSuffixes = []string{
	"blazeredirector.ea.com",
	"gosredirector.ea.com",
}

// shouldTunnelSNI reports whether a connection for this server name must be
// handed to the real EA rather than terminated here.
// It is OFF by default. Tunnelling does satisfy pinning, but it also means EA's
// redirector answers with EA's OWN Blaze address, and the game goes there
// instead of to this server — which broke the full build, whose working route
// depended on the redirector failing and falling back to Blaze-by-IP here. It
// did not rescue the trial build either. Enable with CYPRESS_CFB27_TLS_TUNNEL=1
// to continue that investigation.
func shouldTunnelSNI(serverName string) bool {
	if os.Getenv("CYPRESS_CFB27_TLS_TUNNEL") != "1" {
		return false
	}
	name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(serverName), "."))
	if name == "" {
		return false
	}
	for _, suffix := range tunnelHostSuffixes {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}

// redirectorPorts are tried in order when tunnelling. Override with
// CYPRESS_CFB27_REDIRECTOR_PORTS (comma separated) if EA moves them.
func redirectorPorts() []string {
	if custom := os.Getenv("CYPRESS_CFB27_REDIRECTOR_PORTS"); custom != "" {
		var ports []string
		for _, part := range strings.Split(custom, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				ports = append(ports, trimmed)
			}
		}
		if len(ports) > 0 {
			return ports
		}
	}
	return []string{"44325", "42127", "443"}
}

var errNoSNI = errors.New("no SNI in ClientHello")

// peekClientHelloSNI reads the server name out of a buffered ClientHello
// WITHOUT consuming it, so the bytes remain available either to the TLS server
// or to the tunnel. crypto/tls exposes the SNI only once the handshake is under
// way, which is already too late to hand the connection off intact.
func peekClientHelloSNI(reader *bufio.Reader) (string, error) {
	header, err := reader.Peek(5)
	if err != nil {
		return "", err
	}
	if header[0] != 0x16 {
		return "", errNoSNI
	}
	recordLength := int(binary.BigEndian.Uint16(header[3:5]))
	if recordLength <= 0 || recordLength > 16384 {
		return "", errNoSNI
	}
	record, err := reader.Peek(5 + recordLength)
	if err != nil {
		return "", err
	}
	return parseSNI(record[5:])
}

// parseSNI walks a ClientHello handshake body to its server_name extension.
func parseSNI(body []byte) (string, error) {
	read := func(n int) ([]byte, bool) {
		if len(body) < n {
			return nil, false
		}
		out := body[:n]
		body = body[n:]
		return out, true
	}
	head, ok := read(4) // handshake type + 24-bit length
	if !ok || head[0] != 0x01 {
		return "", errNoSNI
	}
	if _, ok = read(2 + 32); !ok { // client_version + random
		return "", errNoSNI
	}
	sessionID, ok := read(1)
	if !ok {
		return "", errNoSNI
	}
	if _, ok = read(int(sessionID[0])); !ok {
		return "", errNoSNI
	}
	suites, ok := read(2)
	if !ok {
		return "", errNoSNI
	}
	if _, ok = read(int(binary.BigEndian.Uint16(suites))); !ok {
		return "", errNoSNI
	}
	compression, ok := read(1)
	if !ok {
		return "", errNoSNI
	}
	if _, ok = read(int(compression[0])); !ok {
		return "", errNoSNI
	}
	extensionsLength, ok := read(2)
	if !ok {
		return "", errNoSNI
	}
	extensions, ok := read(int(binary.BigEndian.Uint16(extensionsLength)))
	if !ok {
		return "", errNoSNI
	}
	for len(extensions) >= 4 {
		kind := binary.BigEndian.Uint16(extensions[0:2])
		size := int(binary.BigEndian.Uint16(extensions[2:4]))
		extensions = extensions[4:]
		if len(extensions) < size {
			return "", errNoSNI
		}
		payload := extensions[:size]
		extensions = extensions[size:]
		if kind != 0x0000 { // server_name
			continue
		}
		if len(payload) < 5 {
			return "", errNoSNI
		}
		// server_name_list length, then entry: type(1) + length(2) + host
		if payload[2] != 0x00 { // host_name
			return "", errNoSNI
		}
		nameLength := int(binary.BigEndian.Uint16(payload[3:5]))
		if len(payload) < 5+nameLength {
			return "", errNoSNI
		}
		return string(payload[5 : 5+nameLength]), nil
	}
	return "", errNoSNI
}

// tunnelToRealHost pipes this connection to the real host, ClientHello included,
// so the game negotiates TLS directly with EA and sees EA's own certificate.
func tunnelToRealHost(serverName string, reader *bufio.Reader, client net.Conn) (int64, int64, error) {
	// The bridge rewrote the destination, so the original port is gone by the time
	// the connection reaches us. The Blaze redirector does not listen on 443 —
	// captures show the game dialling it on 44325 — so try the redirector ports
	// first and fall back to 443.
	var upstream net.Conn
	var err error
	for _, port := range redirectorPorts() {
		upstream, err = net.DialTimeout("tcp", net.JoinHostPort(serverName, port), 5*time.Second)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0, 0, err
	}
	defer upstream.Close()
	// Clear any deadline set while sniffing: this connection now lives as long as
	// the game needs it.
	_ = client.SetDeadline(time.Time{})

	// Wait for BOTH directions. Returning after the first one closes the
	// connection while the other side is still talking — which tore the tunnel
	// down during the handshake with EA and looked like the tunnel had simply
	// achieved nothing.
	var sent, received int64
	done := make(chan error, 2)
	go func() {
		n, copyErr := io.Copy(upstream, reader)
		sent = n // reader holds the buffered ClientHello
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- copyErr
	}()
	go func() {
		n, copyErr := io.Copy(client, upstream)
		received = n
		if tcp, ok := client.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- copyErr
	}()
	first := <-done
	second := <-done
	if first != nil {
		return sent, received, first
	}
	return sent, received, second
}
