package cfb27blaze

import "time"

/*
gRPC queries answered with an empty success rather than UNIMPLEMENTED.

These are read-only lookups against EA services the private server does not
stand in for. Returning UNIMPLEMENTED made the client treat each as a service
outage; EntityStatistics/GetView in particular produced "Unable to retrieve your
progression information from EA Servers" and tore down the H2H session right
after the game was set up.

An empty success says what is actually true offline: the query ran, and there is
no data. Anything requiring real content — the friends list used by "Invite a
Friend" — needs a real response and is listed here only so it stops erroring.

Paths are lowercase; the handler lowercases before lookup.
*/
var emptyOKGRPCMethods = map[string]bool{
	// Player progression / stats. GetView is followed by ListStatsByView, so
	// answering only the first left the lookup half-done.
	"/eadp.stats.entitystatistics/getview":         true,
	"/eadp.stats.entitystatistics/liststatsbyview": true,

	// Social: friends, presence, privacy, feeds.
	//
	// Presence matters more than it looks. The client creates a presence session
	// and then connects to it; answering the connect with UNIMPLEMENTED put it in
	// a hot retry loop — 3,400 create/connect pairs in one session — and with no
	// presence, friends never come online, so there is nobody to invite.
	"/eadp.friends.v1.friends/listfriends":                                true,
	"/eadp.friends.v1.favorites/listfavoritefriends":                      true,
	"/eadp.friends.v1.invites/listreceivedfriendinvites":                  true,
	"/eadp.friends.v1.invites/listsentfriendinvites":                      true,
	"/eadp.friends.v2.friendsnotificationsservice/streamnotifications":    true,
	"/eadp.social.privacy.v1.block/listblockedplayers":                    true,
	"/eadp.social.privacy.v1.mute/listmutedplayers":                       true,
	"/eadp.social.presence.v1.presenceservice/createpresencesession":      true,
	"/eadp.social.presence.v1.presenceservice/connecttopresencesession":   true,
	"/eadp.social.presence.v1.presenceservice/subscribetofriendspresence": true,
	// The social overlay re-subscribes and updates its own presence as it opens.
	// Leaving these unimplemented made it fail outright with "Sorry. Something
	// prevented us from fulfilling your request."
	"/eadp.social.presence.v1.presenceservice/subscribetoplayerspresence":                     true,
	"/eadp.social.presence.v1.presenceservice/partialupdatepresencesession":                   true,
	"/eadp.friendrecommendations.v1.friendrecommendations/listfriendrecommendationsstreaming": true,
	"/eadp.errors.v1.attachmentscollectorservice/submitattachment":                            true,
	"/eadp.candi.entitlement.v2.service.entitlement/listentitlements":                         true,
	"/eadp.crossplaycontentmart.v1.crossplaycontentmartservice/getproduct":                    true,
	"/shadowbroker.grpc.service.subscription.usersubscriptionservice/retrievesubscriptions":   true,
	"/eadp.errors.v1.collectorservice/submitcrashreport":                                      true,
	"/eadp.candi.offer.v2.service.offer/listsubscriptionoffers":                               true,
	"/eadp.candi.catalog.v2.service.catalog/listoffers":                                       true,
	"/eadp.feeds.reader.v1.readerservice/getevents":                                           true,

	// The "Invite a Friend" screen renders one player card per row; with these
	// unimplemented the rows stay as empty placeholders.
	"/eadp.playercard.v1.playercardservice/getplayercard":                                     true,
	"/eadp.playercard.v1.playercardservice/batchgetplayercards":                               true,
	"/eadp.social.gameinvite.v1.gameinviteservice/subscribetoreceivedgameinvitenotifications": true,

	// Telemetry sinks that only need acknowledging.
	"/eadp.candi.valuetransfer.v2.service.valuetransfer/transfer": true,
	"/eadp.errors.v1.collectorservice/submitbootsession":          true,
	"/eadp.errors.v1.collectorservice/submitserversession":        true,
}

/*
Server-streaming methods: hold the response open instead of ending it.

These subscribe to a stream of events. Ending the call immediately — whether
with UNIMPLEMENTED or with an empty success — makes the client reconnect at
once, which turns into a hot loop: 3,400 attempts per session while they
returned UNIMPLEMENTED, and 26,000 once they returned success instantly.

Offline there are no events to deliver, so the honest behaviour is what a real
server does on an idle subscription: accept it and stay quiet. The call is held
until the client goes away or the hold expires, which keeps the client calm and
costs one blocked goroutine per subscription.
*/
var streamingGRPCMethods = map[string]bool{
	"/eadp.friendrecommendations.v1.friendrecommendations/listfriendrecommendationsstreaming": true,
	"/eadp.social.presence.v1.presenceservice/connecttopresencesession":                       true,
	"/eadp.social.presence.v1.presenceservice/subscribetofriendspresence":                     true,
	"/eadp.friends.v2.friendsnotificationsservice/streamnotifications":                        true,
	"/eadp.social.gameinvite.v1.gameinviteservice/subscribetoreceivedgameinvitenotifications": true,
}

// grpcStreamHold is how long an idle subscription is kept open. Long enough that
// the client is not reconnecting constantly, short enough that goroutines from a
// departed client are reclaimed promptly.
const grpcStreamHold = 5 * time.Minute
