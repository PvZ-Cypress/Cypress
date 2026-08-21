package cfb27blaze

import (
	"bytes"
	"context"
	"testing"
	"time"

	"cypress-servers/internal/blaze"
)

// findIntPath is the test-side reader for the nested leaves the H2H setup
// rewrites.
func findIntPath(t *testing.T, fields []blaze.Field, path ...string) int64 {
	t.Helper()
	value, ok := findInt(fields, path...)
	if !ok {
		t.Fatalf("field %v is missing", path)
	}
	return value
}

// frameCollector lets a test read what the server pushed to a connection.
// WriteFrame emits the header, metadata and payload as separate writes, so the
// bytes are accumulated and frames are lifted out as they complete.
type frameCollector struct {
	buffer *bytes.Buffer
	frames chan blaze.Frame
}

func newFrameCollector() *frameCollector {
	return &frameCollector{buffer: &bytes.Buffer{}, frames: make(chan blaze.Frame, 8)}
}

func (c *frameCollector) Write(p []byte) (int, error) {
	c.buffer.Write(p)
	for {
		snapshot := c.buffer.Bytes()
		frame, err := blaze.ReadFrame(bytes.NewReader(snapshot))
		if err != nil {
			return len(p), nil
		}
		consumed := blaze.HeaderSize + int(frame.Header.MetadataSize) + len(frame.Payload)
		c.buffer.Next(consumed)
		select {
		case c.frames <- frame:
		default:
		}
	}
}

// The captured NotifyGameSetup describes an EA-hosted dedicated game server. We
// have none, so the setup must be re-pointed at the host player: same topology
// change, same address, same connection group, everywhere it appears. Getting
// this wrong is what made both clients aim 4/65 at connection group
// 862947450767 and give up twelve seconds later.
func TestGameSetupAdvertisesThePeerHostedTopology(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	game := &gameSession{id: 12345678}
	host := &gamePlayer{
		identity:        clientIdentity{blazeID: 1001, nucleusID: 9001, personaID: 1001, personaName: "LocalPlayer"},
		externalIP:      0x6473C85A,
		externalPort:    3659,
		slot:            0,
		connectionGroup: 5551001,
		userSessionID:   7771001,
	}
	guest := &gamePlayer{
		identity:        clientIdentity{blazeID: 1002, nucleusID: 9002, personaID: 1002, personaName: "SecondPlayer"},
		externalIP:      0x644D4B44,
		externalPort:    3659,
		slot:            1,
		connectionGroup: 5551002,
		userSessionID:   7771002,
	}

	frame, err := svc.buildGameSetupNotification(game, []*gamePlayer{host, guest})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	fields, err := blaze.Decode(frame.Payload)
	if err != nil {
		t.Fatalf("built frame does not decode: %v", err)
	}
	gameFields, ok := childFields(fields, "GAME")
	if !ok {
		t.Fatal("no GAME struct")
	}

	if got := findIntPath(t, gameFields, "NTOP"); got != gameNetworkPeerHosted {
		t.Errorf("NTOP = %d, want %d (peer-hosted)", got, gameNetworkPeerHosted)
	}
	// The host player runs the network now, so both the platform host and the
	// topology host are them, and the dedicated-server slot is empty.
	for _, tag := range []string{"PHST", "THST"} {
		if got := findIntPath(t, gameFields, tag, "CONG"); got != host.connectionGroup {
			t.Errorf("%s/CONG = %d, want the host's %d", tag, got, host.connectionGroup)
		}
		if got := findIntPath(t, gameFields, tag, "HPID"); got != host.identity.personaID {
			t.Errorf("%s/HPID = %d, want %d", tag, got, host.identity.personaID)
		}
	}
	if got := findIntPath(t, gameFields, "DHST", "CONG"); got != 0 {
		t.Errorf("DHST/CONG = %d, want 0 — there is no dedicated server", got)
	}
	// The captured address and, just as importantly, its port must be gone: the
	// EA server answered on 21030, and pointing a peer at <player>:21030 is as
	// unreachable as pointing it at EA.
	for _, tag := range []string{"HNET", "DNET"} {
		list, ok := findList(gameFields, tag)
		if !ok || len(list.Values) == 0 {
			t.Fatalf("%s is missing", tag)
		}
		union, ok := list.Values[0].(blaze.Union)
		if !ok {
			t.Fatalf("%s[0] is %T, want a union", tag, list.Values[0])
		}
		if got := findIntPath(t, union.Members, "IP"); got != int64(host.externalIP) {
			t.Errorf("%s IP = %d, want the host's %d", tag, got, host.externalIP)
		}
		if got := findIntPath(t, union.Members, "PORT"); got != 3659 {
			t.Errorf("%s PORT = %d, want the game port 3659", tag, got)
		}
	}
	admin, ok := findList(gameFields, "ADMN")
	if !ok || len(admin.Values) != 1 || admin.Values[0] != host.identity.personaID {
		t.Errorf("ADMN = %v, want just the host %d", admin.Values, host.identity.personaID)
	}
}

// Every player needs their own mesh identity in the roster. Cloning one captured
// entry gave both players the same CONG, so neither could tell the other apart
// from itself.
func TestGameSetupRosterGivesEachPlayerTheirOwnMeshIdentity(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	game := &gameSession{id: 12345678}
	players := []*gamePlayer{
		{
			identity:        clientIdentity{blazeID: 1001, nucleusID: 9001, personaID: 1001, personaName: "LocalPlayer"},
			externalIP:      0x6473C85A,
			externalPort:    3659,
			slot:            0,
			connectionGroup: 5551001,
			userSessionID:   7771001,
		},
		{
			identity:        clientIdentity{blazeID: 1002, nucleusID: 9002, personaID: 1002, personaName: "SecondPlayer"},
			externalIP:      0x644D4B44,
			externalPort:    3659,
			slot:            1,
			connectionGroup: 5551002,
			userSessionID:   7771002,
		},
	}

	frame, err := svc.buildGameSetupNotification(game, players)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	fields, _ := blaze.Decode(frame.Payload)
	roster, ok := findList(fields, "PROS")
	if !ok || len(roster.Values) != 2 {
		t.Fatalf("roster has %d entries, want 2", len(roster.Values))
	}

	seenGroups := map[int64]bool{}
	seenUUIDs := map[string]bool{}
	for i, value := range roster.Values {
		entry := value.([]blaze.Field)
		player := players[i]
		if got := findIntPath(t, entry, "CONG"); got != player.connectionGroup {
			t.Errorf("roster[%d] CONG = %d, want the group the client asked for, %d", i, got, player.connectionGroup)
		}
		if seenGroups[player.connectionGroup] {
			t.Errorf("roster[%d] repeats connection group %d", i, player.connectionGroup)
		}
		seenGroups[player.connectionGroup] = true

		var uuid string
		var group blaze.ObjectID
		for _, field := range entry {
			switch field.Tag {
			case "UUID":
				uuid, _ = field.Value.(string)
			case "UGID":
				group, _ = field.Value.(blaze.ObjectID)
			}
		}
		if group.ID != player.connectionGroup {
			t.Errorf("roster[%d] UGID = %d, want %d", i, group.ID, player.connectionGroup)
		}
		if uuid == "" || seenUUIDs[uuid] {
			t.Errorf("roster[%d] UUID %q is empty or shared with another player", i, uuid)
		}
		seenUUIDs[uuid] = true

		// EXID is the Nucleus id, and it must agree with AIDS/EAID/NID.
		if got := findIntPath(t, entry, "EXID"); got != player.identity.nucleusID {
			t.Errorf("roster[%d] EXID = %d, want the Nucleus id %d", i, got, player.identity.nucleusID)
		}
		if got := findIntPath(t, entry, "AIDS", "EAID", "NID"); got != player.identity.nucleusID {
			t.Errorf("roster[%d] AIDS/EAID/NID = %d, want %d", i, got, player.identity.nucleusID)
		}
		// Seats are one-based on the wire.
		if got := findIntPath(t, entry, "SID"); got != player.slot+1 {
			t.Errorf("roster[%d] SID = %d, want %d", i, got, player.slot+1)
		}
		for _, half := range []string{"EXIP", "INIP"} {
			if got := findIntPath(t, entry, "PNET", "VALU", half, "IP"); got != int64(player.externalIP) {
				t.Errorf("roster[%d] %s IP = %d, want %d", i, half, got, player.externalIP)
			}
			if got := findIntPath(t, entry, "PNET", "VALU", half, "PORT"); got != 3659 {
				t.Errorf("roster[%d] %s PORT = %d, want 3659", i, half, got)
			}
		}
	}
}

// NotifyPlayerJoining introduces a player to the others, so it has to describe
// them the same way the game setup does. It used to say CONG was the Blaze id
// while the setup said it was the captured group — two different answers to the
// one question the other client is asking.
func TestPlayerJoiningAgreesWithTheGameSetup(t *testing.T) {
	player := &gamePlayer{
		identity:        clientIdentity{blazeID: 1002, nucleusID: 9002, personaID: 1002, personaName: "SecondPlayer"},
		externalIP:      0x644D4B44,
		externalPort:    3659,
		slot:            1,
		connectionGroup: 5551002,
		userSessionID:   7771002,
	}
	frame, err := buildPlayerJoiningNotification(12345678, player)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	fields, err := blaze.Decode(frame.Payload)
	if err != nil {
		t.Fatalf("built frame does not decode: %v", err)
	}
	data, ok := childFields(fields, "PDAT")
	if !ok {
		t.Fatal("no PDAT")
	}
	if got := findIntPath(t, data, "CONG"); got != player.connectionGroup {
		t.Errorf("CONG = %d, want %d", got, player.connectionGroup)
	}
	if got := findIntPath(t, data, "EXID"); got != player.identity.nucleusID {
		t.Errorf("EXID = %d, want the Nucleus id %d", got, player.identity.nucleusID)
	}
	if got := findIntPath(t, data, "PNET", "VALU", "EXIP", "PORT"); got != 3659 {
		t.Errorf("EXIP PORT = %d, want 3659", got)
	}
	if bytes.Contains(frame.Payload, []byte("waddwadd")) {
		t.Error("the captured persona is still in the payload")
	}
}

// The connection group and the game port come out of the client's own request;
// the TCP source port is an ephemeral one and is not what the game listens on.
func TestCreateOrJoinTakesTheMeshDetailsFromTheRequest(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "CGD", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "PNET", Type: blaze.TypeUnion, Value: blaze.Union{
				ActiveMember: 2,
				Value: &blaze.Field{Tag: "VALU", Type: blaze.TypeStruct, Value: []blaze.Field{
					{Tag: "INIP", Type: blaze.TypeStruct, Value: []blaze.Field{
						{Tag: "IP", Type: blaze.TypeInteger, Value: int64(167772311)},
						{Tag: "PORT", Type: blaze.TypeInteger, Value: int64(3659)},
					}},
				}},
			}},
		}},
		{Tag: "PJD", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "BTPL", Type: blaze.TypeObjectID, Value: blaze.ObjectID{
				Type: blaze.ObjectType{Component: ComponentUserSessions, Type: 2},
				ID:   862947450767,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := withClientSession(context.Background(), &clientSession{
		identity:     clientIdentity{blazeID: 1001, nucleusID: 9001, personaID: 1001, personaName: "LocalPlayer"},
		externalIP:   0x6473C85A,
		externalPort: 53273, // the ephemeral TCP source port, which is not the game port
	})
	player := svc.playerFromRequest(ctx, blaze.Frame{Payload: payload})

	if player.connectionGroup != 862947450767 {
		t.Errorf("connection group = %d, want the one in BTPL", player.connectionGroup)
	}
	if player.externalPort != 3659 {
		t.Errorf("game port = %d, want 3659 from the request's PNET, not the TCP source port", player.externalPort)
	}
	if player.externalIP != 0x6473C85A {
		t.Errorf("address = %d, want the one the client connected from", player.externalIP)
	}

	// A request we cannot read must still leave a usable player rather than a
	// zero connection group.
	fallback := svc.playerFromRequest(ctx, blaze.Frame{Payload: []byte{0xff}})
	if fallback.connectionGroup == 0 || fallback.externalPort != defaultGamePort {
		t.Errorf("fallback player = group %d port %d, want a non-zero group on port %d",
			fallback.connectionGroup, fallback.externalPort, defaultGamePort)
	}
}

// A player who leaves must lose their seat. Acknowledging 4/22 silently left the
// registry full of ghosts, and the next arrival was paired with one of them
// instead of with the person they meant to play.
func TestRemovePlayerFreesTheSeat(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	host := &gamePlayer{
		identity:        clientIdentity{blazeID: 1001, personaID: 1001, personaName: "LocalPlayer"},
		playerKey:       1,
		connectionGroup: 5551001,
	}
	guest := &gamePlayer{
		identity:        clientIdentity{blazeID: 1002, personaID: 1002, personaName: "SecondPlayer"},
		playerKey:       2,
		connectionGroup: 5551002,
	}
	game, _ := svc.games.createOrJoin(host)
	svc.games.createOrJoin(guest)
	if got := len(svc.games.playersOf(game.id)); got != 2 {
		t.Fatalf("game has %d players, want 2", got)
	}

	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: game.id},
		{Tag: "PID", Type: blaze.TypeInteger, Value: int64(1002)},
		{Tag: "REAS", Type: blaze.TypeInteger, Value: int64(8)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.handleRemovePlayer(context.Background(), blaze.Frame{Payload: payload})

	remaining := svc.games.playersOf(game.id)
	if len(remaining) != 1 || remaining[0].identity.personaID != 1001 {
		t.Fatalf("after 4/22 the game holds %d players, want only the host", len(remaining))
	}

	// The seat that opened up must be reusable, and the new player must get it
	// rather than being seated behind the departed one.
	third := &gamePlayer{
		identity:        clientIdentity{blazeID: 1003, personaID: 1003, personaName: "ThirdPlayer"},
		playerKey:       3,
		connectionGroup: 5551003,
	}
	rejoined, _ := svc.games.createOrJoin(third)
	if rejoined.id != game.id {
		t.Fatalf("new player started game %d instead of joining %d", rejoined.id, game.id)
	}
	if third.slot != 1 {
		t.Errorf("new player took slot %d, want the freed slot 1", third.slot)
	}
}

// 4/4 changes the game's settings, and the notification that follows is not the
// request echoed back: EA sends {ATTR, GID}, where ATTR is the new settings word.
func TestGameSettingChangeIsRelayedInTheCapturedShape(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	host := &gamePlayer{
		identity:     clientIdentity{blazeID: 1001, personaID: 1001, personaName: "LocalPlayer"},
		playerKey:    1,
		connectionID: "c-000001",
	}
	game, _ := svc.games.createOrJoin(host)

	collector := newFrameCollector()
	svc.frameWriters.Store("c-000001", &connWriter{w: collector})

	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: game.id},
		{Tag: "GSET", Type: blaze.TypeInteger, Value: int64(1072)},
		{Tag: "GSTM", Type: blaze.TypeInteger, Value: int64(2063)},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.handleGameSettingUpdate(context.Background(), blaze.Frame{Payload: payload})

	select {
	case frame := <-collector.frames:
		if frame.Header.Command != NotifyGameAttribChange {
			t.Fatalf("relayed 4/%d, want 4/110", frame.Header.Command)
		}
		fields, err := blaze.Decode(frame.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if got := findIntPath(t, fields, "ATTR"); got != 1072 {
			t.Errorf("ATTR = %d, want the new settings word 1072", got)
		}
		if got := findIntPath(t, fields, "GID"); got != game.id {
			t.Errorf("GID = %d, want %d", got, game.id)
		}
		if _, ok := findInt(fields, "GSTM"); ok {
			t.Error("the request was echoed instead of rebuilt: GSTM has no place in 4/110")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no 4/110 was relayed")
	}
}

// 4/177 records the caller's external session, and EA answers with the session
// it now holds — including the game id. It used to answer with nothing.
func TestExternalSessionUpdateReportsTheJoinedGame(t *testing.T) {
	svc := NewService(Config{Profile: "LocalPlayer"})
	cs := &clientSession{identity: clientIdentity{blazeID: 1001, personaID: 1001}}
	cs.activeGame.Store(12345678)

	fields, code := svc.handleExternalSessionUpdate(withClientSession(context.Background(), cs), blaze.Frame{})
	if code != 0 {
		t.Fatalf("4/177 returned error 0x%X", code)
	}
	if got := findIntPath(t, fields, "USES", "UGID"); got != 12345678 {
		t.Errorf("USES/UGID = %d, want the joined game", got)
	}
	if got := findIntPath(t, fields, "PSES", "UGID"); got != 0 {
		t.Errorf("PSES/UGID = %d, want 0 — it is the session being replaced", got)
	}
}
