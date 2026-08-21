package cfb27blaze

import (
	"context"
	"time"

	"cypress-servers/internal/blaze"
)

/*
Relay lobby changes between the players in a game.

Everything a player does in the lobby — picking a team, readying up, changing a
client status — is sent to the server as an attribute update (4/8). The server's
job is to tell everyone else. Acknowledging the update and saying nothing left
each client talking to itself: both players sat on the away side, neither saw the
other's team, and readying up did nothing at all because the other side was never
told it had happened.

The request and the notification carry exactly the same fields, so the relay is a
straight echo of the payload:

	4/8  request      { ATTR map, GID, PID }
	4/90 notification { ATTR map, GID, PID }

Confirmed against the capture, where every client attribute update is followed by
a 4/90 to the players in that game (26 of them in one Play-a-Friend session).
*/

const (
	// NotifyGamePlayerAttribChange carries one player's attribute change.
	NotifyGamePlayerAttribChange uint16 = 90
	// NotifyGameStateChange carries a game-state transition (GSTA). This is what
	// drives the lobby forward: without it the client sits in the lobby forever.
	NotifyGameStateChange uint16 = 100
	// NotifyGameAttribChange carries a change to the game's own attributes.
	NotifyGameAttribChange uint16 = 110
	// NotifyPlayerStateChange carries a player's state (STAT). STAT=4 marks a
	// player as a full, active member of the game.
	NotifyPlayerStateChange uint16 = 116
	// NotifyPlayerJoinCompleted confirms a player has finished joining (TIME).
	NotifyPlayerJoinCompleted uint16 = 30

	// playerStateActive is the STAT value for a fully-joined player.
	playerStateActive int64 = 4
	// GSTA transitions on the way to a match: the setup snapshot carries
	// INITIALIZING, the server pushes PRE_GAME once the game exists, and the host
	// advances to IN_GAME when it launches. Every capture shows 4/100 GSTA=130
	// straight after NotifyGameSetup, then 131.
	gameStateInitializing int64 = 1
	gameStatePreGame      int64 = 130
	gameStateInGame       int64 = 131
)

// gameManagerNotification builds a component-4 notification from fields.
func gameManagerNotification(command uint16, fields []blaze.Field) (blaze.Frame, error) {
	payload, err := blaze.Encode(fields)
	if err != nil {
		return blaze.Frame{}, err
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     command,
			MessageType: blaze.MessageTypeNotification,
		},
		Payload: payload,
	}, nil
}

// gameStateNotification builds 4/100 for a game-state transition.
func gameStateNotification(gameID int64, state int64) (blaze.Frame, error) {
	return gameManagerNotification(NotifyGameStateChange, []blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "GSTA", Type: blaze.TypeInteger, Value: state},
	})
}

// playerStateNotification builds 4/116 marking a player's state.
func playerStateNotification(gameID int64, personaID int64, state int64) (blaze.Frame, error) {
	return gameManagerNotification(NotifyPlayerStateChange, []blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "PID", Type: blaze.TypeInteger, Value: personaID},
		{Tag: "STAT", Type: blaze.TypeInteger, Value: state},
	})
}

// playerJoinCompletedNotification builds 4/30 confirming a player joined.
func playerJoinCompletedNotification(gameID int64, personaID int64, when time.Time) (blaze.Frame, error) {
	return gameManagerNotification(NotifyPlayerJoinCompleted, []blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "PID", Type: blaze.TypeInteger, Value: personaID},
		{Tag: "TIME", Type: blaze.TypeInteger, Value: when.UnixMicro()},
	})
}

// gameIDFromRequest reads the GID a lobby request refers to.
func gameIDFromRequest(request blaze.Frame) (int64, bool) {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return 0, false
	}
	for _, field := range fields {
		if field.Tag == "GID" {
			if id, ok := field.Value.(int64); ok {
				return id, true
			}
		}
	}
	return 0, false
}

// broadcastToGame sends a frame to every player in a game.
//
// The change is sent to the player who made it as well: the client expects its
// own update to be confirmed by the server rather than assuming it applied.
func (s *Service) broadcastToGame(gameID int64, frame blaze.Frame) {
	for _, player := range s.games.playersOf(gameID) {
		if player.connectionID == "" {
			continue
		}
		s.sendToConnection(player.connectionID, frame)
	}
}

// handleGameAttributeUpdate answers 4/8 and relays it to the other players.
func (s *Service) handleGameAttributeUpdate(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	gameID, ok := gameIDFromRequest(request)
	if !ok {
		// Without a game there is nobody to tell; acknowledge and move on.
		return nil, 0
	}
	// The notification body is the request body, so it is echoed rather than
	// rebuilt — nothing to get wrong, and unknown attributes pass through intact.
	notification := blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     NotifyGamePlayerAttribChange,
			MessageType: blaze.MessageTypeNotification,
		},
		Payload: append([]byte(nil), request.Payload...),
	}
	go func() {
		// Let the acknowledgement reach the client first, matching the order the
		// capture shows.
		time.Sleep(50 * time.Millisecond)
		s.broadcastToGame(gameID, notification)
	}()
	return nil, 0
}

// handleSetGameState answers 4/3 and broadcasts the new game state as 4/100.
//
// This is the launch driver. When a player readies up, the host client pushes
// the game state forward with 4/3; the server must tell everyone with 4/100, or
// the lobby never advances. The capture shows exactly this: 4/3 GSTA=131 in,
// 4/100 GSTA=131 out to all players.
func (s *Service) handleSetGameState(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	gameID, ok := gameIDFromRequest(request)
	if !ok {
		return nil, 0
	}
	notification := blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     NotifyGameStateChange,
			MessageType: blaze.MessageTypeNotification,
		},
		Payload: append([]byte(nil), request.Payload...),
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.broadcastToGame(gameID, notification)
	}()
	return nil, 0
}

// announceMembership tells everyone in a game that a player is a full, active
// member and has finished joining. Without these the other side sees the joiner
// as a shell — present in the roster but never marked ready, so the match can
// never start.
func (s *Service) announceMembership(gameID int64, personaID int64) {
	now := time.Now().UTC()
	if frame, err := playerStateNotification(gameID, personaID, playerStateActive); err == nil {
		s.broadcastToGame(gameID, frame)
	}
	if frame, err := playerJoinCompletedNotification(gameID, personaID, now); err == nil {
		s.broadcastToGame(gameID, frame)
	}
}

// handleGameSettingUpdate answers 4/4 and relays the game's own settings change.
//
// Unlike the player-attribute relay this is not an echo: the request is
// {GID, GSET, GSTM} but the notification is {ATTR, GID}, where ATTR is the new
// settings word. Echoing the request sent the client a frame with two fields it
// was not looking for and none of the one it was.
func (s *Service) handleGameSettingUpdate(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	gameID, ok := gameIDFromRequest(request)
	if !ok {
		return nil, 0
	}
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, 0
	}
	settings, ok := findInt(fields, "GSET")
	if !ok {
		return nil, 0
	}
	notification, err := gameManagerNotification(NotifyGameAttribChange, []blaze.Field{
		{Tag: "ATTR", Type: blaze.TypeInteger, Value: settings},
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
	})
	if err != nil {
		return nil, 0
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.broadcastToGame(gameID, notification)
	}()
	return nil, 0
}
