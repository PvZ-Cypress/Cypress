package cfb27blaze

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

/*
Local sign-in for the trial build.

The two CFB27 builds authenticate differently, which is why the same private
server worked for one machine and not the other:

  - the full build calls gRPC accounts.grpc.ea.com TokenService/…, which this
    server already answers with a stub, and then proceeds to Blaze;
  - the trial build uses the older HTTP OAuth flow (/connect/auth,
    /connect/token), which nothing here implemented.

Proxying that flow to the real EA (see auth_passthrough.go) does complete the
exchange — real 302, real token, successful refresh — but the game still stops
before Blaze. The likely reason is that it then holds a genuine EA token while
every other service it talks to is this fake server. So the flow is answered
locally instead, consistent with the token the gRPC path hands the full build.

Set CYPRESS_CFB27_AUTH_PROXY=1 to restore the upstream proxy for comparison.
*/

func authProxyEnabled() bool {
	return os.Getenv("CYPRESS_CFB27_AUTH_PROXY") == "1"
}

// localAuthCode is the authorization code handed back from /connect/auth and
// accepted by /connect/token. A fixed value is fine: this server is the only
// issuer and the only verifier.
const localAuthCode = "cypress-local-authorization-code"

const (
	localAccessToken  = "cypress-local-access-token"
	localRefreshToken = "cypress-local-refresh-token"
	localTokenExpiry  = 14400
)

// localAuthRedirect answers /connect/auth. The OAuth "code" flow expects a 302
// back to redirect_uri carrying the code; the game follows it itself, which is
// why this must not be collapsed into a 200.
func localAuthRedirect(request *http.Request) (int, http.Header, []byte) {
	query := request.URL.Query()
	redirect := query.Get("redirect_uri")
	header := http.Header{}
	if redirect == "" {
		// Without a redirect_uri there is nowhere to send the code; answering
		// with the token shape directly is the best available fallback.
		body, _ := json.Marshal(map[string]any{"code": localAuthCode})
		header.Set("Content-Type", "application/json")
		return http.StatusOK, header, body
	}
	separator := "?"
	if strings.Contains(redirect, "?") {
		separator = "&"
	}
	location := redirect + separator + "code=" + url.QueryEscape(localAuthCode)
	if state := query.Get("state"); state != "" {
		location += "&state=" + url.QueryEscape(state)
	}
	header.Set("Location", location)
	return http.StatusFound, header, nil
}

// localTokenResponse answers /connect/token for both the authorization-code and
// refresh-token grants.
func localTokenResponse() (int, http.Header, []byte) {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]any{
		"access_token":  localAccessToken,
		"token_type":    "Bearer",
		"expires_in":    localTokenExpiry,
		"refresh_token": localRefreshToken,
		"id_token":      nil,
	})
	return http.StatusOK, header, body
}

// localTokenInfo answers /connect/tokeninfo, which is how the game turns a token
// into the identity it will present to Blaze. The ids match the ones this server
// uses everywhere else, so the session is consistent end to end.
func localTokenInfo() (int, http.Header, []byte) {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]any{
		"client_id":     "OOA",
		"expires_in":    localTokenExpiry,
		"persona_id":    fmt.Sprintf("%d", localPersonaID),
		"pid_id":        fmt.Sprintf("%d", localNucleusAccountID),
		"pid_type":      "NUCLEUS",
		"user_id":       fmt.Sprintf("%d", localNucleusAccountID),
		"scope":         "basic.identity basic.persona basic.entitlement",
		"is_underage":   false,
		"authenticator": "NUCLEUS",
	})
	return http.StatusOK, header, body
}

// localAuthResponse answers an EA sign-in request locally. The bool reports
// whether this request was handled here.
func localAuthResponse(request *http.Request) (int, http.Header, []byte, bool) {
	switch strings.ToLower(request.URL.Path) {
	case "/connect/auth":
		status, header, body := localAuthRedirect(request)
		return status, header, body, true
	case "/connect/token":
		status, header, body := localTokenResponse()
		return status, header, body, true
	case "/connect/tokeninfo":
		status, header, body := localTokenInfo()
		return status, header, body, true
	case "/connect/logout":
		return http.StatusOK, http.Header{}, []byte("{}"), true
	}
	return 0, nil, nil, false
}
