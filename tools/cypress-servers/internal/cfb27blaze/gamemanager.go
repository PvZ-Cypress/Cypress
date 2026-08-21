package cfb27blaze

import (
	"context"
	_ "embed"
	"net"
	"sync"
	"time"

	"cypress-servers/internal/blaze"
)

// Play-a-Friend / H2H support (Blaze GameManager, component 4).
//
// Wire shapes come from real two-player captures (2026-08-08/09). Those captures
// show EA running H2H as CLIENT_SERVER_DEDICATED (NTOP 1): the game setup points
// both clients at an EA-hosted game server on AWS, and neither player carries the
// match. We have no such server, so the game is re-advertised as peer-hosted
// (NTOP 0) with one of the players as the topology host. On a VPN both peers are
// directly routable, so that works without NAT traversal.
//
// Two things identify a player on the game network and both must be per-player:
//
//	CONG  the connection group — who a client is connecting *to* (4/65 TCG) and
//	      reporting QoS *from* (4/171 LCID). The client picks its own and states
//	      it in the request (PJD/BTPL on 4/10, PLJD/BTPL on 4/9); the server's job
//	      is to hand it back out to the other players, not to invent one.
//	PNET  the address a peer dials. The client's own PNET only carries its LAN
//	      address (EXIP is zero without EA's QoS probe), so the routable half is
//	      taken from the socket it connected on and the port from its PNET — 3659.
//
// Before this, both fields were left at the values baked into the captured
// NotifyGameSetup, so every client was told its opponent lived in connection
// group 862947450767 at an EA address. Both then aimed 4/65 at the same
// non-existent group, timed out after twelve seconds and sent 4/22.
const (
	CommandGameManagerCreateGame     uint16 = 3 // set game state in capture (GSTA)
	CommandGameManagerSetGameState   uint16 = 3
	CommandGameManagerSetGameSetting uint16 = 4
	CommandGameManagerUpdateAttrs    uint16 = 8
	CommandGameManagerConnectQoS     uint16 = 65
	CommandGameManagerQoSReport      uint16 = 171
	CommandGameManagerMeshEndpoint   uint16 = 177
	CommandGameManagerRemovePlayer   uint16 = 22
	CommandGameManagerLeaveGroup     uint16 = 67
	NotifyGameManagerPlayerJoining   uint16 = 21
	NotifyGameManagerPlayerRemoved   uint16 = 40

	// gameNetworkPeerHosted is Blaze's CLIENT_SERVER_PEER_HOSTED topology: one of
	// the players hosts the match. The captures all show 1 (CLIENT_SERVER_DEDICATED)
	// because EA allocates a server; we cannot, so the setup is rewritten to 0.
	gameNetworkPeerHosted int64 = 0
	// defaultGamePort is the UDP port CFB27 plays on. Every capture and every
	// client PNET carries 3659; it is only a fallback for when the request omits it.
	defaultGamePort int64 = 3659
)

// gamePlayer is one participant in an H2H game.
type gamePlayer struct {
	identity     clientIdentity
	playerKey    uint32
	connectionID string
	slot         int64
	// externalIP/port are what the peer dials: the address the client connected
	// from (routable on the VPN) paired with the game port it listens on.
	externalIP   uint32
	externalPort int64
	// connectionGroup is the client's own mesh identity, taken from BTPL in its
	// request. Every other player is told to connect to this.
	connectionGroup int64
	// userSessionID stands in for Blaze's HSES/UID. Its value does not matter to
	// the client; being distinct per player and consistent between the game's host
	// fields and the roster does.
	userSessionID int64
}

// hostRef fills one of the {CONG, CSID, HPID, HSES, HSLT} host structs
// (PHST/THST/DHST/EOBS/EOWN) that name who runs the game network.
func (p *gamePlayer) hostRef() []blaze.Field {
	return []blaze.Field{
		{Tag: "CONG", Type: blaze.TypeInteger, Value: p.connectionGroup},
		{Tag: "CSID", Type: blaze.TypeInteger, Value: p.slot + 1},
		{Tag: "HPID", Type: blaze.TypeInteger, Value: p.identity.personaID},
		{Tag: "HSES", Type: blaze.TypeInteger, Value: p.userSessionID},
		{Tag: "HSLT", Type: blaze.TypeInteger, Value: p.slot},
	}
}

type gameSession struct {
	id      int64
	players []*gamePlayer
}

type gameRegistry struct {
	mu     sync.Mutex
	games  map[int64]*gameSession
	nextID int64
}

// capturedGameID was the game id baked into the captured NotifyGameSetup, which
// game ids used to have to match because the payload could not be decoded and so
// was replayed whole. It is now rewritten when the frame is built, so this is
// only a starting point for the counter — any value would do.
const capturedGameID int64 = 3954783540361

func newGameRegistry() *gameRegistry {
	return &gameRegistry{games: map[int64]*gameSession{}, nextID: capturedGameID - 1}
}

// createOrJoin puts the caller in the first game that has room, otherwise starts
// a new one. Play-a-Friend is two-player, so "room" means fewer than two.
func (r *gameRegistry) createOrJoin(player *gamePlayer) (*gameSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A player already in a game is re-issuing create-or-join on another of their
	// sockets; return the existing game rather than seating them twice.
	for _, game := range r.games {
		for _, seated := range game.players {
			if seated.playerKey == player.playerKey {
				// Keep the seat, but take the fresh request's mesh details: a client
				// that re-issues create-or-join may have rebuilt its connection group.
				seated.connectionID = player.connectionID
				seated.connectionGroup = player.connectionGroup
				seated.externalIP = player.externalIP
				seated.externalPort = player.externalPort
				return game, false
			}
		}
	}
	for _, game := range r.games {
		if len(game.players) < 2 {
			player.slot = int64(len(game.players))
			game.players = append(game.players, player)
			return game, false
		}
	}
	r.nextID++
	game := &gameSession{id: r.nextID}
	player.slot = 0
	game.players = append(game.players, player)
	r.games[game.id] = game
	return game, true
}

// playersOf snapshots everyone in a game, in seat order.
func (r *gameRegistry) playersOf(gameID int64) []*gamePlayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	game, ok := r.games[gameID]
	if !ok {
		return nil
	}
	return append([]*gamePlayer(nil), game.players...)
}

func (r *gameRegistry) peersOf(gameID int64, exclude uint32) []*gamePlayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	game, ok := r.games[gameID]
	if !ok {
		return nil
	}
	var peers []*gamePlayer
	for _, p := range game.players {
		if p.playerKey != exclude {
			peers = append(peers, p)
		}
	}
	return peers
}

// removeFromGame takes one player out of one game, by persona rather than by
// address: 4/22 names the player who left, not the socket it arrived on.
func (r *gameRegistry) removeFromGame(gameID int64, personaID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	game, ok := r.games[gameID]
	if !ok {
		return
	}
	kept := game.players[:0]
	for _, p := range game.players {
		if p.identity.personaID != personaID {
			kept = append(kept, p)
		}
	}
	game.players = kept
	if len(game.players) == 0 {
		delete(r.games, gameID)
		return
	}
	// Seats are positional — the host is seat zero — so close the gap.
	for i, p := range game.players {
		p.slot = int64(i)
	}
}

func (r *gameRegistry) remove(playerKey uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, game := range r.games {
		kept := game.players[:0]
		for _, p := range game.players {
			if p.playerKey != playerKey {
				kept = append(kept, p)
			}
		}
		game.players = kept
		if len(game.players) == 0 {
			delete(r.games, id)
		}
	}
}

// ipv4ToUint32 converts a dotted address to the integer form Blaze uses (the
// capture carries 3232235981 for 192.168.1.13).
func ipv4ToUint32(addr net.Addr) (uint32, int64) {
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, 0
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return 0, 0
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, 0
	}
	var port int64
	for _, c := range portText {
		if c < '0' || c > '9' {
			break
		}
		port = port*10 + int64(c-'0')
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3]), port
}

// findInt reads an integer leaf at a struct path, e.g. "CGD","PNET","VALU",
// "INIP","PORT". A union on the path is followed through its members.
func findInt(fields []blaze.Field, path ...string) (int64, bool) {
	if len(path) == 0 {
		return 0, false
	}
	if len(path) == 1 {
		for _, field := range fields {
			if field.Tag == path[0] {
				value, ok := field.Value.(int64)
				return value, ok
			}
		}
		return 0, false
	}
	child, ok := childFields(fields, path[0])
	if !ok {
		return 0, false
	}
	return findInt(child, path[1:]...)
}

// findObjectID reads an object-id leaf (BTPL carries the connection group).
func findObjectID(fields []blaze.Field, path ...string) (int64, bool) {
	if len(path) == 0 {
		return 0, false
	}
	if len(path) == 1 {
		for _, field := range fields {
			if field.Tag == path[0] {
				if id, ok := field.Value.(blaze.ObjectID); ok {
					return id.ID, true
				}
			}
		}
		return 0, false
	}
	child, ok := childFields(fields, path[0])
	if !ok {
		return 0, false
	}
	return findObjectID(child, path[1:]...)
}

// playerFromRequest builds the participant for a create-or-join / join request.
//
// The two commands carry the same information under different tags — 4/10 puts
// the create data in CGD and the join data in PJD, 4/9 uses CMGD and PLJD — so
// both spellings are tried.
func (s *Service) playerFromRequest(ctx context.Context, request blaze.Frame) *gamePlayer {
	cs := s.clientSessionFrom(ctx)
	// The reply and its notifications must travel on the socket that asked, not on
	// whichever of the game's ~18 sockets happens to be newest.
	connectionID := connectionIDFrom(ctx)
	if connectionID == "" {
		connectionID = cs.currentConnectionID()
	}
	player := &gamePlayer{
		identity:     cs.identity,
		playerKey:    cs.playerKey,
		connectionID: connectionID,
		externalIP:   cs.externalIP,
		externalPort: defaultGamePort,
		// The client states its own connection group; these are only used when the
		// request could not be decoded at all.
		connectionGroup: cs.identity.blazeID,
		userSessionID:   cs.identity.blazeID,
	}
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return player
	}
	for _, tag := range []string{"PJD", "PLJD"} {
		if group, ok := findObjectID(fields, tag, "BTPL"); ok && group != 0 {
			player.connectionGroup = group
			break
		}
	}
	// The client's own PNET names the port it listens on. Its EXIP is zero without
	// EA's QoS probe, so only the port is taken from here — the address comes from
	// the socket, which on the VPN is what the peer must dial.
	for _, tag := range []string{"CGD", "CMGD"} {
		if port, ok := findInt(fields, tag, "PNET", "VALU", "INIP", "PORT"); ok && port > 0 {
			player.externalPort = port
			break
		}
	}
	return player
}

// handleGameManagerCreateOrJoin answers 4/10. The reply shape is taken verbatim
// from the capture: external-session ids (empty for PC), the game id, and the
// join-game-status / call flags.
func (s *Service) handleGameManagerCreateOrJoinGame(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	player := s.playerFromRequest(ctx, request)
	requestConn := player.connectionID
	game, _ := s.games.createOrJoin(player)
	cs.activeGame.Store(game.id)

	// The client shows "PLEASE WAIT..." until it receives the game-setup
	// notification that follows create-or-join; the reply alone is not enough.
	// It is built from the captured frame with this session's identity
	// substituted in — replaying the capture verbatim crashed the client,
	// because it could not find itself in the captured roster.
	go func(connectionID string, gameID int64, joiner *gamePlayer) {
		// Deliver after the create-or-join reply has gone out, so the client sees
		// them in the order the capture shows.
		time.Sleep(150 * time.Millisecond)
		if frame, err := s.buildGameSetupNotification(game, s.games.playersOf(gameID)); err == nil {
			s.sendToConnection(connectionID, frame)
			// The capture follows the setup with the transition into PRE_GAME; the
			// lobby will not progress without it.
			if state, err := gameStateNotification(gameID, gameStatePreGame); err == nil {
				s.sendToConnection(connectionID, state)
			}
		} else {
			s.record(Event{
				Time:         time.Now().UTC(),
				RunID:        s.config.RunID,
				ConnectionID: connectionID,
				Status:       "gamesetup-build-failed",
				DecodeError:  err.Error(),
			})
		}
		s.broadcastPlayerJoining(gameID, joiner)
	}(requestConn, game.id, player)

	emptyString := func(tag string) blaze.Field {
		return blaze.Field{Tag: tag, Type: blaze.TypeString, Value: ""}
	}
	return []blaze.Field{
		{Tag: "ESID", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "PS4", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("NPSI")}},
			{Tag: "PS5", Type: blaze.TypeStruct, Value: []blaze.Field{
				{Tag: "MATC", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("MAID"), emptyString("MAOI")}},
				{Tag: "PSES", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("PSID")}},
			}},
			{Tag: "XBX", Type: blaze.TypeStruct, Value: []blaze.Field{
				{Tag: "MPAU", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("CONS")}},
			}},
		}},
		{Tag: "GID", Type: blaze.TypeInteger, Value: game.id},
		{Tag: "JGS", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "OCAL", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

// playerJoiningNotification builds 4/21 — the frame that carries a player's
// identity and, crucially, the network address the peer will dial.
func playerJoiningNotification(gameID int64, p *gamePlayer) (blaze.Frame, error) {
	address := func(tag string) blaze.Field {
		return blaze.Field{Tag: tag, Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "IP", Type: blaze.TypeInteger, Value: int64(p.externalIP)},
			{Tag: "MACI", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "PORT", Type: blaze.TypeInteger, Value: p.externalPort},
		}}
	}
	playerData := []blaze.Field{
		{Tag: "AIDS", Type: blaze.TypeStruct, Value: localAccountIdentityFields(p.identity)},
		{Tag: "BLOB", Type: blaze.TypeBlob, Value: []byte(nil)},
		{Tag: "CNTY", Type: blaze.TypeInteger, Value: localCountry},
		{Tag: "CONG", Type: blaze.TypeInteger, Value: p.identity.blazeID},
		{Tag: "CSID", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "DSUI", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "ENCR", Type: blaze.TypeString, Value: ""},
		{Tag: "EXID", Type: blaze.TypeInteger, Value: p.identity.nucleusID},
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "JFPS", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "JVMM", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "LOC", Type: blaze.TypeInteger, Value: localLocale},
		{Tag: "NAME", Type: blaze.TypeString, Value: p.identity.personaName},
		{Tag: "NASP", Type: blaze.TypeString, Value: "cem_ea_id"},
		{Tag: "PID", Type: blaze.TypeInteger, Value: p.identity.personaID},
		// PNET union member 2 = the internal/external address pair the capture uses.
		{Tag: "PNET", Type: blaze.TypeUnion, Value: blaze.Union{
			ActiveMember: 2,
			Value: &blaze.Field{Tag: "VALU", Type: blaze.TypeStruct, Value: []blaze.Field{
				address("EXIP"),
				address("INIP"),
			}},
		}},
		{Tag: "SID", Type: blaze.TypeInteger, Value: p.slot},
		{Tag: "SLOT", Type: blaze.TypeInteger, Value: p.slot},
		{Tag: "STAT", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TIDX", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "UID", Type: blaze.TypeInteger, Value: p.identity.blazeID},
	}
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "PDAT", Type: blaze.TypeStruct, Value: playerData},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: p.identity.blazeID},
		{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     NotifyGameManagerPlayerJoining,
			MessageType: blaze.MessageTypeNotification,
		},
		Metadata: metadata,
		Payload:  payload,
	}, nil
}

// broadcastPlayerJoining tells existing players about the joiner and the joiner
// about them, so both sides learn the address they need to dial.
func (s *Service) broadcastPlayerJoining(gameID int64, joiner *gamePlayer) {
	peers := s.games.peersOf(gameID, joiner.playerKey)
	for _, peer := range peers {
		if frame, err := buildPlayerJoiningNotification(gameID, joiner); err == nil {
			s.sendToConnection(peer.connectionID, frame)
		}
		if frame, err := buildPlayerJoiningNotification(gameID, peer); err == nil {
			s.sendToConnection(joiner.connectionID, frame)
		}
	}
	// Mark everyone in the game as a full, active member. The capture sends a
	// 4/116 STAT=4 and 4/30 per player after the joins, and without them the
	// other side never treats the joiner as ready — the match cannot start.
	for _, player := range s.games.playersOf(gameID) {
		s.announceMembership(gameID, player.identity.personaID)
	}
}

// handleGameManagerAck answers the game-state / QoS traffic (4/65, 4/171 and
// friends). The capture shows these replying with an empty success; they are
// state the client pushes, not data it needs back.
func handleGameManagerAck(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return nil, 0
}

// externalSessionIDs is the empty per-platform external-session block. PC has no
// external session, but the client still reads the structure.
func externalSessionIDs() []blaze.Field {
	emptyString := func(tag string) blaze.Field {
		return blaze.Field{Tag: tag, Type: blaze.TypeString, Value: ""}
	}
	return []blaze.Field{
		{Tag: "PS4", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("NPSI")}},
		{Tag: "PS5", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "MATC", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("MAID"), emptyString("MAOI")}},
			{Tag: "PSES", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("PSID")}},
		}},
		{Tag: "XBX", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "MPAU", Type: blaze.TypeStruct, Value: []blaze.Field{emptyString("CONS")}},
		}},
	}
}

// handleExternalSessionUpdate answers 4/177. EA returns the caller's external
// session state — the previous one (empty) and the new one, whose UGID is the
// game just joined. It was answered with nothing before, which left the client
// without the session it had just asked the server to record.
func (s *Service) handleExternalSessionUpdate(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	session := func(gameID int64, activity bool) []blaze.Field {
		fields := []blaze.Field{
			{Tag: "PUSH", Type: blaze.TypeString, Value: ""},
		}
		if activity {
			fields = append(fields, blaze.Field{Tag: "UACT", Type: blaze.TypeMap, Value: blaze.Map{
				KeyType:   blaze.TypeInteger,
				ValueType: blaze.TypeInteger,
				Entries:   []blaze.MapEntry{{Key: int64(1), Value: int64(1)}},
			}})
		}
		return append(fields,
			blaze.Field{Tag: "UGID", Type: blaze.TypeInteger, Value: gameID},
			blaze.Field{Tag: "USID", Type: blaze.TypeStruct, Value: externalSessionIDs()},
			blaze.Field{Tag: "UTID", Type: blaze.TypeInteger, Value: int64(0)},
		)
	}
	return []blaze.Field{
		// PSES is the session being replaced, so it is always the empty one.
		{Tag: "PSES", Type: blaze.TypeStruct, Value: session(0, false)},
		{Tag: "USEI", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "USES", Type: blaze.TypeStruct, Value: session(cs.activeGame.Load(), true)},
	}, 0
}

// playerRemovedNotification builds 4/40, which tells the remaining players that
// somebody has gone.
func playerRemovedNotification(gameID int64, personaID int64, reason int64) (blaze.Frame, error) {
	return gameManagerNotification(NotifyGameManagerPlayerRemoved, []blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "GID", Type: blaze.TypeInteger, Value: gameID},
		{Tag: "LFPJ", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PID", Type: blaze.TypeInteger, Value: personaID},
		{Tag: "REAS", Type: blaze.TypeInteger, Value: reason},
	})
}

// handleRemovePlayer answers 4/22 and actually takes the player out of the game.
//
// This used to be a silent acknowledgement, so a player who left stayed seated
// forever: the registry filled up with ghosts and the next real player was
// paired with one of them instead of with their opponent.
func (s *Service) handleRemovePlayer(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, 0
	}
	gameID, ok := findInt(fields, "GID")
	if !ok {
		return nil, 0
	}
	personaID, ok := findInt(fields, "PID")
	if !ok {
		personaID = cs.identity.personaID
	}
	reason, _ := findInt(fields, "REAS")

	// Tell the others before the seat disappears, so the notification still goes
	// to everyone who was in the game with them.
	if frame, err := playerRemovedNotification(gameID, personaID, reason); err == nil {
		s.broadcastToGame(gameID, frame)
	}
	s.games.removeFromGame(gameID, personaID)
	if cs.activeGame.Load() == gameID {
		cs.activeGame.Store(0)
	}
	return nil, 0
}

func (s *Service) registerGameManagerHandlers() {
	s.handlers[route{ComponentGameManager, CommandGameManagerCreateOrJoin}] = s.handleGameManagerCreateOrJoinGame
	// Lobby changes must reach the other players; acknowledging them silently
	// left each client talking to itself. See lobby_relay.go.
	s.handlers[route{ComponentGameManager, CommandGameManagerUpdateAttrs}] = s.handleGameAttributeUpdate
	s.handlers[route{ComponentGameManager, CommandGameManagerSetGameSetting}] = s.handleGameSettingUpdate
	// 4/3 drives the game state forward (the ready-up → launch transition); it
	// must be broadcast as 4/100, not acknowledged silently.
	s.handlers[route{ComponentGameManager, CommandGameManagerSetGameState}] = s.handleSetGameState
	// A player leaving must free their seat, or the registry accumulates ghosts
	// and pairs the next arrival with one.
	s.handlers[route{ComponentGameManager, CommandGameManagerRemovePlayer}] = s.handleRemovePlayer
	// 4/177 carries the caller's external session back; EA answers it with 192
	// bytes, not with nothing.
	s.handlers[route{ComponentGameManager, CommandGameManagerMeshEndpoint}] = s.handleExternalSessionUpdate
	for _, command := range []uint16{
		9, 12, 16,
		CommandGameManagerConnectQoS, // 65
		66,
		CommandGameManagerLeaveGroup, // 67
		105,
		CommandGameManagerQoSReport, // 171
	} {
		s.handlers[route{ComponentGameManager, command}] = handleGameManagerAck
	}
}

//go:embed fixtures/gm-20-notify.bin
var gameSetupNotifyPayload []byte

// gameSetupNotification is NotifyGameSetup (4/20) — the frame the client waits
// for after create-or-join before it will leave "PLEASE WAIT...". It is replayed
// from the capture: the payload is a 1.7 KB structure this decoder cannot fully
// parse yet, so replaying it verbatim is more faithful than synthesising it. The
// game id inside it matches capturedGameID.
func gameSetupNotification() (blaze.Frame, error) {
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: LocalBlazeID},
		{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     20,
			MessageType: blaze.MessageTypeNotification,
		},
		Metadata: metadata,
		Payload:  append([]byte(nil), gameSetupNotifyPayload...),
	}, nil
}
