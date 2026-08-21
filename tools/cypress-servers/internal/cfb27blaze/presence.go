package cfb27blaze

import (
	"fmt"
	"time"
)

/*
Presence for players on this server.

The friends screen decides who can be invited from presence, not from the friends
list: a player listed but not present shows as offline and offers no actions.
EA cannot supply this — the players are connected here, so EA correctly reports
them offline.

The wire shape was recorded through the gRPC proxy while a friend was genuinely
online on EA (fixtures/grpc-presence-chunk-03.bin):

	PresenceEvent {
	  PresenceUpdate update = 1 {
	    PlayerId player_id = 1 { string id = 1 }
	    int32    status    = 2
	    Session  session   = 4 {
	      string    session_id = 1
	      int32     state      = 2
	      Timestamp updated    = 3 { int64 seconds = 1, int32 nanos = 2 }
	      Client    client     = 4 { {"EA"}, {"r1eu"}, {"PC"}, "en-us" }
	      repeated Attribute attributes = 7 {
	        string key = 1
	        Value  value = 2 { string text = 1, int64 number = 3 }
	      }
	    }
	  }
	}

The attribute that decides the green dot is photon_status = "ONLINE".
*/

const (
	presenceStatusOnline   = 1
	presenceAttributeText  = 1
	presenceAttributeInt   = 3
	presenceProductID      = "r1eu"
	presencePlatform       = "PC"
	presenceLocale         = "en-us"
	presenceStatusAttr     = "photon_status"
	presenceCrossplayAttr  = "photon_crossplayEnabled"
	presenceMessagingAttr  = "photon_messagingSupport"
	presenceOnlineValue    = "ONLINE"
	presenceMessagingLevel = 2
)

// presenceAttributeString builds one string-valued presence attribute.
func presenceAttributeString(key string, value string) []byte {
	attribute := protobufStringField(1, key)
	attribute = append(attribute, protobufBytesField(2, protobufStringField(presenceAttributeText, value))...)
	return protobufBytesField(7, attribute)
}

// presenceAttributeNumber builds one numeric presence attribute.
func presenceAttributeNumber(key string, value uint64) []byte {
	attribute := protobufStringField(1, key)
	attribute = append(attribute, protobufBytesField(2, protobufVarintField(presenceAttributeInt, value))...)
	return protobufBytesField(7, attribute)
}

// presenceSession describes a player's live session.
func presenceSession(player connectedPlayer, now time.Time) []byte {
	client := protobufBytesField(1, protobufStringField(1, "EA"))
	client = append(client, protobufBytesField(2, protobufStringField(1, presenceProductID))...)
	client = append(client, protobufBytesField(3, protobufStringField(1, presencePlatform))...)
	client = append(client, protobufStringField(4, presenceLocale)...)

	updated := protobufVarintField(1, uint64(now.Unix()))
	updated = append(updated, protobufVarintField(2, uint64(now.Nanosecond()))...)

	session := protobufStringField(1, fmt.Sprintf("%d:cypress-local-session", player.personaID))
	session = append(session, protobufVarintField(2, 1)...)
	session = append(session, protobufBytesField(3, updated)...)
	session = append(session, protobufBytesField(4, client)...)
	session = append(session, presenceAttributeNumber(presenceCrossplayAttr, 1)...)
	session = append(session, presenceAttributeNumber(presenceMessagingAttr, presenceMessagingLevel)...)
	session = append(session, presenceAttributeString(presenceStatusAttr, presenceOnlineValue)...)
	return session
}

// presenceEvent is one "this player is online" message.
func presenceEvent(player connectedPlayer, now time.Time) []byte {
	update := protobufBytesField(1, protobufStringField(1, fmt.Sprintf("%d", player.personaID)))
	update = append(update, protobufVarintField(2, presenceStatusOnline)...)
	update = append(update, protobufBytesField(4, presenceSession(player, now))...)
	return grpcFrame(protobufBytesField(1, update))
}

// presenceEventsFor builds one online event per player, which is what the
// friends screen needs before it will offer any action against them.
func presenceEventsFor(players []connectedPlayer, now time.Time) [][]byte {
	events := make([][]byte, 0, len(players))
	for _, player := range players {
		events = append(events, presenceEvent(player, now))
	}
	return events
}

// createPresenceSessionResponse answers CreatePresenceSession with a session id
// for the caller, matching the "<id>:<token>" form EA uses.
func createPresenceSessionResponse(personaID int64) []byte {
	return grpcFrame(protobufStringField(1, fmt.Sprintf("%d:cypress-local-session", personaID)))
}
