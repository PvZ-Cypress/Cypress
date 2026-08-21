package cfb27blaze

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

/*
Forward gRPC calls to the real EA and record what comes back.

Observation mode (CYPRESS_CFB27_PROXY_ALL=1) forwarded HTTP/1.1 only, so every
gRPC call still hit a local stub — which is why running the whole session
against EA still showed our own empty answers for the friends list and
progression.

These services cannot be synthesised by guesswork: ListFriends returns protobuf
whose shape we have no capture of, because the ProtoSSL hook records Blaze
traffic rather than WinHTTP. Forwarding the call and logging the reply gives the
exact bytes to build from — the same route that made NotifyGameSetup work.

The response body is recorded as hex so a reply can be lifted straight out of the
run log and turned into a fixture.

Only active in observation mode. It sends the player's real account traffic to
EA, so it is opt-in and off by default.
*/

// grpcUpstreamClient speaks HTTP/2, which gRPC requires. ForceAttemptHTTP2 gets
// this from the standard library without pulling in x/net.
var grpcUpstreamClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		DialContext:         (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout: 15 * time.Second,
	},
}

type grpcProxyResult struct {
	status  int
	header  http.Header
	trailer http.Header
	body    []byte
}

// proxyGRPC forwards one gRPC call upstream and returns the real response,
// including the trailers that carry grpc-status.
func (s *Service) proxyGRPC(realHost string, request *http.Request, body []byte) (grpcProxyResult, error) {
	target := "https://" + hostname(realHost) + request.URL.RequestURI()
	upstream, err := http.NewRequest(request.Method, target, bytes.NewReader(body))
	if err != nil {
		return grpcProxyResult{}, err
	}
	for name, values := range request.Header {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "proxy-connection", "transfer-encoding",
			"upgrade", "host", "content-length":
			continue
		}
		for _, value := range values {
			upstream.Header.Add(name, value)
		}
	}
	// gRPC requires this to receive trailers, and grpc-status arrives as one.
	upstream.Header.Set("TE", "trailers")
	upstream.Host = hostname(realHost)

	response, err := grpcUpstreamClient.Do(upstream)
	if err != nil {
		return grpcProxyResult{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return grpcProxyResult{}, err
	}
	// Trailers are only populated once the body has been consumed.
	return grpcProxyResult{
		status:  response.StatusCode,
		header:  response.Header,
		trailer: response.Trailer,
		body:    responseBody,
	}, nil
}

// writeProxiedGRPC relays an upstream gRPC response to the game.
func writeProxiedGRPC(writer http.ResponseWriter, result grpcProxyResult) {
	header := writer.Header()
	for name, values := range result.header {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "transfer-encoding", "content-length":
			continue
		}
		for _, value := range values {
			header.Add(name, value)
		}
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/grpc")
	}
	header.Set("Trailer", "Grpc-Status, Grpc-Message")
	writer.WriteHeader(result.status)
	if len(result.body) > 0 {
		_, _ = writer.Write(result.body)
	}
	status := result.trailer.Get("Grpc-Status")
	if status == "" {
		status = "0"
	}
	header.Set("Grpc-Status", status)
	if message := result.trailer.Get("Grpc-Message"); message != "" {
		header.Set("Grpc-Message", message)
	}
}

/*
Streaming relay.

Presence and notification subscriptions stay open and deliver events as they
happen. Buffering the whole body — the right thing for a unary call — would hold
them until the client timeout and lose the events entirely, which are precisely
what we need to learn the shape of an "online" presence update.

Each chunk is relayed to the game as it arrives and recorded separately, so a
single presence event can be lifted out of the run log.
*/
func (s *Service) proxyGRPCStream(
	realHost string,
	request *http.Request,
	body []byte,
	writer http.ResponseWriter,
	onChunk func(chunk []byte),
) error {
	target := "https://" + hostname(realHost) + request.URL.RequestURI()
	upstream, err := http.NewRequestWithContext(request.Context(), request.Method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for name, values := range request.Header {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "proxy-connection", "transfer-encoding",
			"upgrade", "host", "content-length":
			continue
		}
		for _, value := range values {
			upstream.Header.Add(name, value)
		}
	}
	upstream.Header.Set("TE", "trailers")
	upstream.Host = hostname(realHost)

	// No client timeout: the point of the stream is to stay open. It ends when
	// the game disconnects, which cancels the request context.
	streamClient := &http.Client{Transport: grpcUpstreamClient.Transport}
	response, err := streamClient.Do(upstream)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	header := writer.Header()
	for name, values := range response.Header {
		switch strings.ToLower(name) {
		case "connection", "keep-alive", "transfer-encoding", "content-length":
			continue
		}
		for _, value := range values {
			header.Add(name, value)
		}
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/grpc")
	}
	header.Set("Trailer", "Grpc-Status, Grpc-Message")
	writer.WriteHeader(response.StatusCode)
	flusher, _ := writer.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	buffer := make([]byte, 32<<10)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			onChunk(chunk)
			if _, writeErr := writer.Write(chunk); writeErr != nil {
				return writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}
	status := response.Trailer.Get("Grpc-Status")
	if status == "" {
		status = "0"
	}
	header.Set("Grpc-Status", status)
	return nil
}
