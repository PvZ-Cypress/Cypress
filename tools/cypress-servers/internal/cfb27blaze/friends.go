package cfb27blaze

import (
	"fmt"
	"sort"
	"time"
)

/*
The friends list is the roster of players on THIS server.

For a multi-user dynasty a player has to be able to pick which of several people
to invite, so the list cannot come from EA: EA has no idea who is connected here.
Proxying it upstream returns the player's real EA friends, every one of them
offline, because they are on this server rather than EA's.

So the list is served from our own session table — everyone currently connected,
minus the player asking. Their ids are the ids this server already issues, which
is what makes an invite addressable: the client names a player id, and we know
exactly which connection that is.

The wire shape was read off a real ListFriends response recorded through the gRPC
proxy (fixtures/grpc-listfriends.bin):

	ListFriendsResponse {
	  repeated Friend friends = 1
	}
	Friend {
	  Identity identity = 1 {
	    PlayerId  player_id    = 1 { string id = 1 }
	    string    persona_name = 2
	    string    display_name = 3
	  }
	  Timestamp friends_since = 2 { int64 seconds = 1, int32 nanos = 2 }
	  Flags     flags         = 3 { int32 value = 1 }
	}
*/

// friendsSince is the "friends since" timestamp reported for every player. The
// value is not meaningful on a private server, but the field must be present.
var friendsSince = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// connectedPlayer is one entry in the server's roster.
type connectedPlayer struct {
	blazeID     int64
	personaID   int64
	personaName string
}

// connectedPlayers lists everyone on the server except the caller, in a stable
// order so the menu does not reshuffle between refreshes.
func (s *Service) connectedPlayers(exclude int64) []connectedPlayer {
	var players []connectedPlayer
	s.playerSessions.Range(func(_, value any) bool {
		cs, ok := value.(*clientSession)
		if !ok || !cs.playing() || cs.identity.personaID == exclude {
			return true
		}
		players = append(players, connectedPlayer{
			blazeID:     cs.identity.blazeID,
			personaID:   cs.identity.personaID,
			personaName: cs.identity.personaName,
		})
		return true
	})
	sort.Slice(players, func(i, j int) bool { return players[i].personaID < players[j].personaID })
	return players
}

// playerIdentity builds the Identity submessage naming one player.
func playerIdentity(player connectedPlayer) []byte {
	playerID := protobufStringField(1, fmt.Sprintf("%d", player.personaID))
	identity := protobufBytesField(1, playerID)
	identity = append(identity, protobufStringField(2, player.personaName)...)
	identity = append(identity, protobufStringField(3, player.personaName)...)
	return identity
}

// listFriendsResponse builds the gRPC reply listing everyone else on the server.
func listFriendsResponse(players []connectedPlayer) []byte {
	var message []byte
	for _, player := range players {
		since := protobufVarintField(1, uint64(friendsSince.Unix()))
		since = append(since, protobufVarintField(2, 0)...)

		friend := protobufBytesField(1, playerIdentity(player))
		friend = append(friend, protobufBytesField(2, since)...)
		friend = append(friend, protobufBytesField(3, protobufVarintField(1, 1))...)

		message = append(message, protobufBytesField(1, friend)...)
	}
	return grpcFrame(message)
}

// callerPersonaFor identifies who is asking. The social calls arrive on their own
// connections rather than the Blaze one, so the caller is resolved by address —
// the same key the session table already uses. Without this every player was
// handed the list built for the host, so a joining player saw themselves in it
// and not the person they wanted to invite.
func (s *Service) callerPersonaFor(remote string) int64 {
	ip, _ := ipv4ToUint32(&stringAddr{remote})
	if value, ok := s.playerSessions.Load(ip); ok {
		if cs, ok := value.(*clientSession); ok {
			return cs.identity.personaID
		}
	}
	return localPersonaID
}

// stringAddr adapts a "host:port" string to net.Addr for ipv4ToUint32.
type stringAddr struct{ value string }

func (a *stringAddr) Network() string { return "tcp" }
func (a *stringAddr) String() string  { return a.value }

// friendsListForRequest is the reply for a ListFriends call from this session.
func (s *Service) friendsListForRequest(callerPersonaID int64) []byte {
	return listFriendsResponse(s.connectedPlayers(callerPersonaID))
}
