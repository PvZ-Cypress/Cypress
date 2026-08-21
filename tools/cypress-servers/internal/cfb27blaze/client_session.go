package cfb27blaze

import (
	"context"
	"cypress-servers/internal/blaze"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// clientSession is the per-connection state that used to live on Service as
// shared atomics. With one connection its values match the old globals exactly;
// with several connections each player keeps an independent identity, selected
// team, and UI-notification sequencing so two people no longer clobber each
// other.
type clientSession struct {
	identity clientIdentity
	// playerKey identifies the player (their address), not a single socket. One
	// game opens ~18 TCP connections; they are all the same person, so identity
	// and dynasty state must be shared across them. Keying per connection made
	// the client look like 18 different players and broke login.
	playerKey uint32
	// lastConnectionID is the most recent socket for this player, used to push
	// server-initiated notifications (H2H peer introductions).
	lastConnectionID atomic.Value // string
	// Address this client connected from. On a VPN this is exactly the address
	// an H2H peer must dial, so it is what we hand to the other player.
	externalIP   uint32
	externalPort int64

	// Live socket count for this player. A game opens many connections and closes
	// them independently, so game membership must only drop at zero — otherwise one
	// closing socket would evict a player mid-match.
	connCount atomic.Int32
	// isPlayer is set once the session carries Blaze traffic. Health checks and
	// HTTP requests also create sessions, and those are not players.
	isPlayer             atomic.Bool
	selectedTeam         atomic.Int64
	activeDynastySession atomic.Int64
	activeGame           atomic.Int64
	dynastyContract      atomic.Uint32
	dynastyHub           atomic.Uint32
	dynastyAdvance       atomic.Uint32
}

// clientIdentity is the Blaze/Nucleus identity presented to one connection.
type clientIdentity struct {
	blazeID     int64
	nucleusID   int64
	personaID   int64
	personaName string
	sessionKey  string
}

// The first connection reproduces the historical local identity exactly, so a
// single-player session is unchanged. Additional connections get distinct ids.
func (s *Service) identityForSequence(seq uint64) clientIdentity {
	if seq <= 1 {
		return clientIdentity{
			blazeID:     LocalBlazeID,
			nucleusID:   localNucleusAccountID,
			personaID:   localPersonaID,
			personaName: s.config.Profile,
			sessionKey:  localSessionKey,
		}
	}
	id := LocalBlazeID + int64(seq) - 1
	return clientIdentity{
		blazeID:     id,
		nucleusID:   id,
		personaID:   id,
		personaName: fmt.Sprintf("%s-%d", s.config.Profile, seq),
		sessionKey:  fmt.Sprintf("%s-%d", localSessionKey, seq),
	}
}

type sessionCtxKey struct{}

type connIDCtxKey struct{}

func withConnectionID(ctx context.Context, connectionID string) context.Context {
	return context.WithValue(ctx, connIDCtxKey{}, connectionID)
}

// connectionIDFrom is the socket the current request arrived on. A notification
// that answers a request must go back on that socket — a game holds ~18 of them,
// and the player's "most recent" one is usually a different, idle connection.
func connectionIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(connIDCtxKey{}).(string); ok {
		return id
	}
	return ""
}

func withClientSession(ctx context.Context, cs *clientSession) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, cs)
}

// clientSessionFrom returns the connection's session, or a transient default so
// handlers never nil-panic (e.g. in unit tests that call HandleFrame directly).
func (s *Service) clientSessionFrom(ctx context.Context) *clientSession {
	if cs, ok := ctx.Value(sessionCtxKey{}).(*clientSession); ok && cs != nil {
		return cs
	}
	return s.fallbackSession()
}

func (s *Service) fallbackSession() *clientSession {
	s.fallbackOnce.Do(func() {
		s.fallback = &clientSession{identity: s.identityForSequence(1)}
	})
	return s.fallback
}

// sessionForPeer returns the session for the player at this address, creating it
// on first sight. Every connection from the same address shares it, so a game's
// many sockets act as one player. The first player seen keeps the historical
// identity, so single-player behaviour is unchanged.
func (s *Service) sessionForPeer(connectionID string, ip uint32, port int64) *clientSession {
	if existing, ok := s.playerSessions.Load(ip); ok {
		cs := existing.(*clientSession)
		cs.connCount.Add(1)
		cs.lastConnectionID.Store(connectionID)
		s.clientSessions.Store(connectionID, cs)
		return cs
	}
	slot := s.nextPlayerSlot.Add(1)
	identity := s.identityForSequence(slot)
	// Players are picked out of a friends list and named in invites, so each one
	// needs a real name rather than "LocalPlayer"; see roster.go.
	identity.personaName = s.playerNameFor(ip, slot)
	cs := &clientSession{
		identity:     identity,
		playerKey:    ip,
		externalIP:   ip,
		externalPort: port,
	}
	cs.lastConnectionID.Store(connectionID)
	// Only the first player resumes the seeded dynasty; a joining player starts
	// clean so they pick their own team.
	cs.activeDynastySession.Store(s.seededSession.Load())
	if slot <= 1 {
		cs.selectedTeam.Store(s.seededTeam.Load())
	}
	cs.connCount.Add(1)
	actual, loaded := s.playerSessions.LoadOrStore(ip, cs)
	cs = actual.(*clientSession)
	if loaded {
		cs.lastConnectionID.Store(connectionID)
	}
	s.clientSessions.Store(connectionID, cs)
	return cs
}

// connWriter is a connection's frame sink plus a lock. Notifications are sent
// from another goroutine than the request loop, and both must serialise on the
// same writer — concurrent writes would interleave and corrupt the stream.
//
// It must hold the TLS wrapper, not the raw socket: writing plaintext frames
// into an encrypted connection is invisible to the client (and corrupts it).
type connWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (c *connWriter) write(frame blaze.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return blaze.WriteFrame(c.w, frame)
}

func (s *Service) registerFrameWriter(connectionID string, w io.Writer) *connWriter {
	cw := &connWriter{w: w}
	s.frameWriters.Store(connectionID, cw)
	return cw
}

func (s *Service) dropFrameWriter(connectionID string) {
	s.frameWriters.Delete(connectionID)
}

// sendToConnection pushes a server-initiated frame (an H2H notification) to a
// specific connection, through that connection's real frame writer.
// It is logged like a request so the run log shows whether a notification
// actually reached the wire — otherwise a silent client is indistinguishable
// from a notification we never sent.
func (s *Service) sendToConnection(connectionID string, frame blaze.Frame) {
	event := Event{
		Time:         time.Now().UTC(),
		RunID:        s.config.RunID,
		ConnectionID: connectionID,
		Status:       "notify",
		Transport:    "tcp",
		Component:    frame.Header.Component,
		Command:      frame.Header.Command,
		MessageID:    frame.Header.MessageID,
		MessageType:  uint8(frame.Header.MessageType),
		BodyBytes:    len(frame.Payload),
	}
	value, ok := s.frameWriters.Load(connectionID)
	if !ok {
		event.Status = "notify-no-writer"
		s.record(event)
		return
	}
	if err := value.(*connWriter).write(frame); err != nil {
		event.Status = "notify-failed"
		event.DecodeError = err.Error()
	}
	s.record(event)
}

func (s *Service) dropClientSession(connectionID string) {
	s.clientSessions.Delete(connectionID)
}

// currentConnectionID is the player's most recent socket, used as the target for
// server-initiated notifications.
func (cs *clientSession) currentConnectionID() string {
	if v, ok := cs.lastConnectionID.Load().(string); ok {
		return v
	}
	return ""
}

// markPlayer records that this session has spoken Blaze, which is what makes it
// a real player rather than an incidental connection.
func (cs *clientSession) markPlayer() {
	cs.isPlayer.Store(true)
}

// playing reports whether this session belongs to a game client.
func (cs *clientSession) playing() bool {
	return cs.isPlayer.Load()
}

// releaseConnection drops one socket for this player and reports how many remain.
// Game membership is only torn down at zero.
func (cs *clientSession) releaseConnection() int32 {
	return cs.connCount.Add(-1)
}
