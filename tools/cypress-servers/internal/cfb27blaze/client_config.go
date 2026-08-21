package cfb27blaze

import (
	"context"
	_ "embed"
	"strings"

	"cypress-servers/internal/blaze"
)

/*
Client configuration (Util 9/1) — including where the roster lives.

The client asks for several named config blocks at boot and takes its service
URLs from them. We answered with a handful of TeamBuilder values and nothing
else, so the game was never told where the roster is and simply never asked for
it — which is why "There was an error downloading the latest player roster"
could not be fixed by retrying, and why no roster request ever reached the
server to be proxied.

OSDK_MADSET carries the data-patch locations:

	DP_BASE_URL     https://eaassets-a.akamaihd.net/gameplayservices/prod/
	                CollegeFootball/2027/datapatch/G5/<hash>/
	DP_CUSTRSTR_URL roster/customroster.eaPatchInfo
	DP_RSTR_URL     roster/roster.eaPatchInfo
	DP_TUNING_URL   tuning/tuning.eaPatchInfo

The captured blocks are replayed as-is. They are service configuration —
URLs, feature flags and limits — with no session or account data in them, and
because the roster file name is resolved through customroster.eaPatchInfo rather
than named directly, replaying this keeps working across roster updates.

Captured 2026-08-13 (proto_20260813_235136_Roster_DL_Invite).
*/

//go:embed fixtures/util-config-osdk_core.bin
var configOSDKCore []byte

//go:embed fixtures/util-config-osdk_client.bin
var configOSDKClient []byte

//go:embed fixtures/util-config-osdk_nucleus.bin
var configOSDKNucleus []byte

//go:embed fixtures/util-config-osdk_abuse_reporting.bin
var configOSDKAbuseReporting []byte

//go:embed fixtures/util-config-osdk_madset.bin
var configOSDKMadset []byte

//go:embed fixtures/util-config-feed.bin
var configFeed []byte

// capturedClientConfigs maps a requested config id to EA's real answer.
func capturedClientConfigs() map[string][]byte {
	return map[string][]byte{
		"osdk_core":            configOSDKCore,
		"osdk_client":          configOSDKClient,
		"osdk_nucleus":         configOSDKNucleus,
		"osdk_abuse_reporting": configOSDKAbuseReporting,
		"osdk_madset":          configOSDKMadset,
		"feed":                 configFeed,
	}
}

// requestedConfigID reads the CFID the client asked for.
func requestedConfigID(request blaze.Frame) string {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return ""
	}
	for _, field := range fields {
		if field.Tag != "CFID" {
			continue
		}
		if id, ok := field.Value.(string); ok {
			return strings.ToLower(strings.TrimSpace(id))
		}
	}
	return ""
}

// handleUtilFetchClientConfigRaw answers with the captured block for the
// requested id. An unknown id falls back to an empty map, which is what the
// client expects for a config it is not entitled to.
func (s *Service) handleUtilFetchClientConfigRaw(_ context.Context, request blaze.Frame) ([]byte, uint16) {
	if payload, ok := capturedClientConfigs()[requestedConfigID(request)]; ok && len(payload) > 0 {
		return payload, 0
	}
	empty, err := blaze.Encode([]blaze.Field{
		{
			Tag:  "CONF",
			Type: blaze.TypeMap,
			Value: blaze.Map{
				KeyType:   blaze.TypeString,
				ValueType: blaze.TypeString,
				Entries:   nil,
			},
		},
	})
	if err != nil {
		return nil, 1
	}
	return empty, 0
}
