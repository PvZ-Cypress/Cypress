package cfb27blaze

import (
	"fmt"
	"time"
)

/*
Player cards — the profile the social UI renders for each player.

"Play a Friend" runs through the EA Connect overlay, and the overlay draws a card
per player. Answering with an empty message is accepted by the transport but
leaves the UI with a card that names nobody, which shows as "the server
encountered an unexpected condition" and, on some paths, takes the client down.

The shape was read off a real BatchGetFriendsPlayerCards recorded through the
gRPC proxy (fixtures/grpc-friendsplayercards.bin):

	PlayerCard {
	  PlayerId player_id = 1 { string id = 1 }
	  repeated Account accounts = 3 {
	    Platform    platform     = 1 { string name = 1 }
	    PlayerId    id           = 2 { string id = 1 }
	    DisplayName display_name = 3 { string value = 1 }
	    Nickname    nickname     = 4 { string value = 1 }
	    Timestamp   since        = 6 { int64 seconds = 1 }
	  }
	  Timestamp updated = 5 { int64 seconds = 1 }
	}

Every player on this server is an "EA" account, which is what the client expects
for a PC player.
*/

const playerCardPlatform = "EA"

// playerCardFor builds one card describing a player.
func playerCardFor(player connectedPlayer, now time.Time) []byte {
	id := protobufStringField(1, fmt.Sprintf("%d", player.personaID))
	seconds := protobufVarintField(1, uint64(now.Unix()))

	account := protobufBytesField(1, protobufStringField(1, playerCardPlatform))
	account = append(account, protobufBytesField(2, id)...)
	account = append(account, protobufBytesField(3, protobufStringField(1, player.personaName))...)
	account = append(account, protobufBytesField(4, protobufStringField(1, player.personaName))...)
	account = append(account, protobufBytesField(6, seconds)...)

	card := protobufBytesField(1, id)
	card = append(card, protobufBytesField(3, account)...)
	card = append(card, protobufBytesField(5, seconds)...)
	return card
}

// playerCardResponse is a single card, for GetPlayerCard.
func playerCardResponse(player connectedPlayer, now time.Time) []byte {
	return grpcFrame(playerCardFor(player, now))
}

// playerCardsResponse is one framed card per player, which is how the captured
// batch reply is structured — a stream of messages rather than one list.
func playerCardsResponse(players []connectedPlayer, now time.Time) []byte {
	var body []byte
	for _, player := range players {
		body = append(body, grpcFrame(playerCardFor(player, now))...)
	}
	return body
}

// cardsForRequest returns cards for everyone on the server, caller included: the
// overlay asks for its own card as well as its friends'.
func (s *Service) cardsForRequest() []byte {
	return playerCardsResponse(s.connectedPlayers(-1), time.Now().UTC())
}

// cardForCaller returns the single card for whoever is asking.
func (s *Service) cardForCaller(remote string) []byte {
	personaID := s.callerPersonaFor(remote)
	for _, player := range s.connectedPlayers(-1) {
		if player.personaID == personaID {
			return playerCardResponse(player, time.Now().UTC())
		}
	}
	// A caller with no session yet still needs a well-formed card.
	return playerCardResponse(connectedPlayer{
		blazeID:     personaID,
		personaID:   personaID,
		personaName: s.config.Profile,
	}, time.Now().UTC())
}
