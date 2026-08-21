package cfb27blaze

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

/*
EA account sign-in cannot be served locally.

Everything else the game asks for is either implemented here or safely stubbed,
but authentication is an OAuth exchange against EA's identity service that we
have no capture of and cannot synthesise. Answering it with a stub produced
"ACCOUNT ERROR" / "lost connection to the EA Servers" — for a while only on a
joining player, and then on the host too once its cached EA session expired.

Two attempts to exempt these hosts inside the bridge failed:

  - resolving their addresses at startup and skipping those at connect(): EA
    fronts them with a CDN, so the game dialled addresses that were never in the
    set we resolved;
  - hooking the game's DNS calls to learn the addresses it actually used:
    WinHTTP resolves inside winhttp.dll rather than through the game's import
    table, so the hook never saw the lookup.

So the redirect stands and the proxying happens here instead. This process is
not hooked, so its own DNS and sockets reach the real internet normally. The
request arrives with the original Host header, which is all we need to forward
it upstream and hand the real response back to the game.
*/

// authPassThroughHosts are served by proxying to the real EA endpoint.
var authPassThroughHosts = []string{
	"accounts.ea.com",
	"signin.ea.com",
}

// hostname strips any port from a Host header value.
func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// authPassThroughPaths are EA's OAuth endpoints.
//
// Matching on the Host header alone does not work, and this was the flaw in the
// first version: the bridge redirects the socket to us, so the game addresses
// the request to the private server ("Host: 100.110.136.90:27920") rather than
// to accounts.ea.com. The path survives that rewrite, so it is what we key on.
var authPassThroughPaths = []string{
	"/connect/auth",
	"/connect/token",
	"/connect/tokeninfo",
	"/connect/logout",
}

func isAuthPassThroughHost(host string) bool {
	name := strings.ToLower(hostname(host))
	for _, candidate := range authPassThroughHosts {
		if name == candidate {
			return true
		}
	}
	return false
}

// isAuthPassThroughRequest reports whether this request is EA sign-in and must
// be proxied upstream instead of answered locally.
func isAuthPassThroughRequest(host string, path string) bool {
	if isAuthPassThroughHost(host) {
		return true
	}
	clean := strings.ToLower(path)
	for _, candidate := range authPassThroughPaths {
		if clean == candidate || strings.HasPrefix(clean, candidate+"/") {
			return true
		}
	}
	return false
}

// upstreamAuthHost is where a proxied auth request is actually sent. When the
// game addressed the request to the private server we must supply EA's real
// hostname ourselves, or the proxy would loop straight back here.
func upstreamAuthHost(host string) string {
	if isAuthPassThroughHost(host) {
		return hostname(host)
	}
	return "accounts.ea.com"
}

// upstreamClient talks to the real EA. It must not follow redirects: the OAuth
// flow signals completion with a 302 whose Location the game itself consumes,
// so following it here would swallow the part the game is waiting for.
var upstreamClient = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

/*
Observation mode: forward everything to the real EA and record what was asked.

The trial build stops right after sign-in without asking us for anything else,
so we cannot tell what it wants. Running the whole session against real EA — it
works there — and logging each request gives the exact sequence to implement.
Routing uses the TLS SNI, not the Host header: the bridge rewrites Host to this
server's address, but the original hostname survives in the handshake.

Enable with CYPRESS_CFB27_PROXY_ALL=1. This sends the session's traffic to EA
under the player's real account, so it is opt-in and off by default.
*/
func proxyAllEnabled() bool {
	return os.Getenv("CYPRESS_CFB27_PROXY_ALL") == "1"
}

// upstreamHostFromSNI returns the hostname to forward to, or "" when the
// connection carries no usable name (an IP literal is not a routable target).
func upstreamHostFromSNI(connection net.Conn) string {
	secure, ok := connection.(*tls.Conn)
	if !ok {
		return ""
	}
	name := secure.ConnectionState().ServerName
	if name == "" || net.ParseIP(name) != nil {
		return ""
	}
	return name
}

// proxyToHost forwards one request to an explicit upstream host.
func (s *Service) proxyToHost(realHost string, request *http.Request, body []byte) (int, http.Header, []byte, error) {
	return s.proxyRequest(realHost, request, body)
}

// proxyToRealEA forwards one request upstream and returns the real response.
func (s *Service) proxyToRealEA(request *http.Request, body []byte) (int, http.Header, []byte, error) {
	return s.proxyRequest(upstreamAuthHost(request.Host), request, body)
}

func (s *Service) proxyRequest(realHost string, request *http.Request, body []byte) (int, http.Header, []byte, error) {
	// Asset downloads are far larger and slower than sign-in responses: a roster
	// database runs to tens of megabytes, well past both the size cap and the
	// short timeout that suit an OAuth exchange.
	limit := int64(8 << 20)
	client := upstreamClient
	asset := isAssetPassThroughHost(realHost)
	if asset {
		limit = assetPassThroughLimit
		client = assetClient
	}
	target := "https://" + realHost + request.URL.RequestURI()
	upstream, err := http.NewRequest(request.Method, target, strings.NewReader(string(body)))
	if err != nil {
		return 0, nil, nil, err
	}
	for name, values := range request.Header {
		// Hop-by-hop headers describe the connection to us, not to EA.
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "proxy-connection", "te", "trailer",
			"transfer-encoding", "upgrade", "host", "content-length":
			continue
		}
		for _, value := range values {
			upstream.Header.Add(name, value)
		}
	}
	upstream.Host = realHost

	response, err := client.Do(upstream)
	if err != nil {
		return 0, nil, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return 0, nil, nil, err
	}
	// Report the SHAPE of EA's answer so the local responder can be made to match
	// it. Only key names and value types are printed: the values are live account
	// credentials and must not be written anywhere. Asset downloads are binary and
	// large, so they are not inspected.
	if !asset {
		logAuthResponseShape(request.URL.Path, responseBody)
	}
	return response.StatusCode, response.Header, responseBody, nil
}

func logAuthResponseShape(path string, body []byte) {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Printf("auth shape %s: %d bytes, not JSON\n", path, len(body))
		return
	}
	keys := make([]string, 0, len(parsed))
	for key, value := range parsed {
		keys = append(keys, fmt.Sprintf("%s:%T", key, value))
	}
	sort.Strings(keys)
	fmt.Printf("auth shape %s: %d bytes, fields = %s\n", path, len(body), strings.Join(keys, ", "))
}

// writeProxiedResponse relays the upstream response verbatim apart from framing.
func writeProxiedResponse(w io.Writer, status int, header http.Header, body []byte) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	for name, values := range header {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "transfer-encoding", "content-length":
			continue
		}
		for _, value := range values {
			fmt.Fprintf(&builder, "%s: %s\r\n", name, value)
		}
	}
	fmt.Fprintf(&builder, "Content-Length: %d\r\nConnection: close\r\n\r\n", len(body))
	if _, err := io.WriteString(w, builder.String()); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}
