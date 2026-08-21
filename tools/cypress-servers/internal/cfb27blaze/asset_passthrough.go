package cfb27blaze

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"
)

/*
Content the private server has no business standing in for.

The roster lives on EA's CDN as a plain file. The game first reads
customroster.eaPatchImpl, which names the current database (mroster_582_172.db
today — the name changes with every roster update), then downloads that file.
Intercepting the CDN and answering with a stub produced "There was an error
downloading the latest player roster", and retrying could never work.

These hosts serve static content, carry no session state, and change on EA's
schedule rather than ours, so requests for them are forwarded to the real host.
That also means the roster stays current automatically: we never have to know
the file name, only to get out of the way.

Unlike sign-in, the Host header survives here — the game reaches these over
WinHTTP with the real hostname — so a host match is enough.
*/
var assetPassThroughSuffixes = []string{
	"eaassets-a.akamaihd.net",
	"gameplayservices.ea.com",
}

// assetPassThroughLimit caps a proxied download. Roster databases run to tens of
// megabytes, far past the limit used for sign-in responses.
const assetPassThroughLimit = 512 << 20

func isAssetPassThroughHost(host string) bool {
	name := strings.ToLower(hostname(host))
	if name == "" {
		return false
	}
	for _, suffix := range assetPassThroughSuffixes {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}

// assetClient downloads CDN content. A roster database is tens of megabytes, so
// it needs a far longer budget than the sign-in exchange allows.
var assetClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:         (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout: 15 * time.Second,
	},
}
