package cfb27blaze

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"cypress-servers/internal/blaze"
)

/*
Build NotifyGameSetup (4/20) from the captured frame as a template.

Replaying the capture verbatim crashes the client: the payload carries the
captured account's roster, and a client that cannot find itself in it dies. The
frame is far too large and too full of unknown fields to synthesise from
scratch, so instead it is decoded, the identity-bearing fields are replaced with
this session's values, and it is re-encoded.

That is only possible because the decoder round-trips this payload exactly — see
TestUnionBodyHoldsSeveralFields and the HNET fix in internal/blaze/tdf.go.

The captured game is CLIENT_SERVER_DEDICATED (NTOP 1) and points at an EA game
server on AWS. We have no such server, so the topology is rewritten to
peer-hosted (NTOP 0) with seat zero as the host: the host's own address and
connection group replace the dedicated server's everywhere they appear.

Fields replaced (found by decoding the capture and following the ids):

	GAME/GID                       the game id
	GAME/NTOP                      peer-hosted instead of dedicated
	GAME/PHST, THST, EOBS[]        the host player; DHST/EOWN cleared, since
	                               there is no dedicated server any more
	GAME/HNET[], DNET[] IP+PORT    the host's address, which peers dial
	GAME/ADMN                      just the host
	PROS[]                         one entry per player, cloned from the template:
	  PID, EXID, NAME, GID         identity and game
	  AIDS/EAID/{NAME,NID,PCID}    EA account identity
	  SID, CSID, TIDX              seat
	  CONG, UGID, UID              mesh identity and session
	  PNET/VALU/{EXIP,INIP}        address and port this player is reached on

CONG is the important one: it is what a client aims 4/65 at, and leaving it at
the captured value told both players their opponent was a connection group that
belonged to neither of them.
*/

// gameNetworkTopology is the NTOP the game is advertised with. Peer-hosted is
// the only topology we can actually serve, but the enum has not been confirmed
// against this build — every capture shows 1, because EA always allocates a
// dedicated server — so CYPRESS_CFB27_GAME_TOPOLOGY overrides it for testing
// without a rebuild.
func gameNetworkTopology() int64 {
	if raw := os.Getenv("CYPRESS_CFB27_GAME_TOPOLOGY"); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return value
		}
	}
	return gameNetworkPeerHosted
}

var (
	gameSetupTemplateOnce   sync.Once
	gameSetupTemplate       []blaze.Field
	gameSetupTemplateErr    error
	gameSetupRosterTemplate []blaze.Field
)

func loadGameSetupTemplate() ([]blaze.Field, []blaze.Field, error) {
	gameSetupTemplateOnce.Do(func() {
		frame, err := gameSetupNotification()
		if err != nil {
			gameSetupTemplateErr = err
			return
		}
		fields, err := blaze.Decode(frame.Payload)
		if err != nil {
			gameSetupTemplateErr = fmt.Errorf("decode captured NotifyGameSetup: %w", err)
			return
		}
		gameSetupTemplate = fields
		roster, ok := findList(fields, "PROS")
		if !ok || len(roster.Values) == 0 {
			gameSetupTemplateErr = fmt.Errorf("captured NotifyGameSetup has no PROS roster")
			return
		}
		entry, ok := roster.Values[0].([]blaze.Field)
		if !ok {
			gameSetupTemplateErr = fmt.Errorf("PROS entry is %T, want a struct", roster.Values[0])
			return
		}
		gameSetupRosterTemplate = entry
	})
	return gameSetupTemplate, gameSetupRosterTemplate, gameSetupTemplateErr
}

// cloneFields deep-copies a decoded tree so the cached template is never
// mutated by a substitution.
func cloneFields(in []blaze.Field) []blaze.Field {
	out := make([]blaze.Field, len(in))
	for i, field := range in {
		out[i] = field
		switch v := field.Value.(type) {
		case []blaze.Field:
			out[i].Value = cloneFields(v)
		case blaze.List:
			values := make([]any, len(v.Values))
			for j, item := range v.Values {
				values[j] = cloneValue(item)
			}
			out[i].Value = blaze.List{ElementType: v.ElementType, Values: values}
		case blaze.Union:
			union := blaze.Union{ActiveMember: v.ActiveMember, Members: cloneFields(v.Members)}
			if len(union.Members) > 0 {
				first := union.Members[0]
				union.Value = &first
			}
			out[i].Value = union
		}
	}
	return out
}

func cloneValue(in any) any {
	switch v := in.(type) {
	case []blaze.Field:
		return cloneFields(v)
	case blaze.Union:
		union := blaze.Union{ActiveMember: v.ActiveMember, Members: cloneFields(v.Members)}
		if len(union.Members) > 0 {
			first := union.Members[0]
			union.Value = &first
		}
		return union
	default:
		return in
	}
}

// childFields returns the sub-fields of a struct or union named tag.
func childFields(fields []blaze.Field, tag string) ([]blaze.Field, bool) {
	for _, field := range fields {
		if field.Tag != tag {
			continue
		}
		switch v := field.Value.(type) {
		case []blaze.Field:
			return v, true
		case blaze.Union:
			return v.Members, true
		}
	}
	return nil, false
}

func findList(fields []blaze.Field, tag string) (blaze.List, bool) {
	for _, field := range fields {
		if field.Tag == tag {
			if list, ok := field.Value.(blaze.List); ok {
				return list, true
			}
		}
	}
	return blaze.List{}, false
}

// setScalar replaces the value of tag in this level only. Reported so a rename
// upstream fails loudly instead of quietly sending the captured value.
func setScalar(fields []blaze.Field, tag string, value any) bool {
	for i := range fields {
		if fields[i].Tag == tag {
			fields[i].Value = value
			return true
		}
	}
	return false
}

// setAtPath walks nested structs/unions and sets a leaf.
func setAtPath(fields []blaze.Field, value any, path ...string) bool {
	if len(path) == 0 {
		return false
	}
	if len(path) == 1 {
		return setScalar(fields, path[0], value)
	}
	child, ok := childFields(fields, path[0])
	if !ok {
		return false
	}
	return setAtPath(child, value, path[1:]...)
}

// setAddressIn rewrites the address inside every union of a network-address list
// (HNET/DNET hold one union carrying an IP and a port).
//
// The port matters as much as the address: the captured value is 21030, the port
// of the EA game server that hosted that match, and pointing a client at
// <peer>:21030 is as unreachable as pointing it at EA.
func setAddressIn(fields []blaze.Field, tag string, ip int64, port int64) bool {
	list, ok := findList(fields, tag)
	if !ok {
		return false
	}
	changed := false
	for _, item := range list.Values {
		union, ok := item.(blaze.Union)
		if !ok {
			continue
		}
		if setScalar(union.Members, "IP", ip) {
			changed = true
		}
		setScalar(union.Members, "PORT", port)
	}
	return changed
}

// setHostRef overwrites one of the {CONG, CSID, HPID, HSES, HSLT} host structs.
func setHostRef(fields []blaze.Field, tag string, ref []blaze.Field) bool {
	target, ok := childFields(fields, tag)
	if !ok {
		return false
	}
	changed := false
	for _, field := range ref {
		if setScalar(target, field.Tag, field.Value) {
			changed = true
		}
	}
	return changed
}

// setHostRefList overwrites the host structs inside a list (EOBS).
func setHostRefList(fields []blaze.Field, tag string, ref []blaze.Field) {
	list, ok := findList(fields, tag)
	if !ok {
		return
	}
	for _, item := range list.Values {
		entry, ok := item.([]blaze.Field)
		if !ok {
			continue
		}
		for _, field := range ref {
			setScalar(entry, field.Tag, field.Value)
		}
	}
}

// playerUUID gives each player a distinct UUID. The captured value is one real
// player's, and handing the same one to both sides of a two-player game is
// asking the client to confuse them.
func playerUUID(player *gamePlayer) string {
	id := uint64(player.identity.personaID)
	group := uint64(player.connectionGroup)
	return fmt.Sprintf("%08x-%04x-4%03x-8%03x-%012x",
		uint32(id), uint16(id>>32), uint16(group)&0x0fff,
		uint16(group>>12)&0x0fff, group&0xffffffffffff)
}

// clearedHostRef is the all-zero host struct the capture uses for EOWN, i.e.
// "nobody". DHST takes it once the dedicated server is gone.
func clearedHostRef() []blaze.Field {
	return []blaze.Field{
		{Tag: "CONG", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CSID", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "HPID", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "HSES", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "HSLT", Type: blaze.TypeInteger, Value: int64(0)},
	}
}

// rosterEntryFor produces one PROS entry for a player.
func rosterEntryFor(template []blaze.Field, game *gameSession, player *gamePlayer) []blaze.Field {
	entry := cloneFields(template)
	identity := player.identity
	address := int64(player.externalIP)

	setScalar(entry, "PID", identity.personaID)
	// EXID is the Nucleus account id, not the Blaze id — in the capture they are
	// 2821938008 and 831257771, and it matches AIDS/EAID/NID.
	setScalar(entry, "EXID", identity.nucleusID)
	setScalar(entry, "NAME", identity.personaName)
	setScalar(entry, "GID", game.id)
	// Seats are one-based on the wire; SLOT is the slot *type* and stays as the
	// template has it.
	setScalar(entry, "SID", player.slot+1)
	setScalar(entry, "CSID", player.slot+1)
	setScalar(entry, "TIDX", player.slot)
	setScalar(entry, "UID", player.userSessionID)
	setScalar(entry, "UUID", playerUUID(player))
	// CONG and UGID are the player's mesh identity: this is the connection group
	// the other clients will try to reach them on.
	setScalar(entry, "CONG", player.connectionGroup)
	setScalar(entry, "UGID", blaze.ObjectID{
		Type: blaze.ObjectType{Component: ComponentUserSessions, Type: 2},
		ID:   player.connectionGroup,
	})

	setAtPath(entry, identity.personaName, "AIDS", "EAID", "NAME")
	setAtPath(entry, identity.nucleusID, "AIDS", "EAID", "NID")
	setAtPath(entry, identity.personaID, "AIDS", "EAID", "PCID")

	// PNET carries the address a peer dials. The client's own external address is
	// unknown to it (EA fills that in from a QoS probe we do not run), so both
	// halves are set to the address it reached us on — on a VPN that is exactly
	// what the peer must dial.
	for _, half := range []string{"EXIP", "INIP"} {
		setAtPath(entry, address, "PNET", "VALU", half, "IP")
		setAtPath(entry, player.externalPort, "PNET", "VALU", half, "PORT")
	}
	return entry
}

// buildGameSetupNotification builds 4/20 for a real game and its players.
func (s *Service) buildGameSetupNotification(game *gameSession, players []*gamePlayer) (blaze.Frame, error) {
	template, rosterTemplate, err := loadGameSetupTemplate()
	if err != nil {
		return blaze.Frame{}, err
	}
	if len(players) == 0 {
		return blaze.Frame{}, fmt.Errorf("game %d has no players", game.id)
	}
	fields := cloneFields(template)

	host := players[0]
	hostAddress := int64(host.externalIP)
	hostRef := host.hostRef()

	gameFields, ok := childFields(fields, "GAME")
	if !ok {
		return blaze.Frame{}, fmt.Errorf("captured NotifyGameSetup has no GAME struct")
	}
	setScalar(gameFields, "GID", game.id)
	setScalar(gameFields, "NTOP", gameNetworkTopology())
	// Seat zero hosts. The platform host was already a player in the capture; the
	// topology host was EA's game server and becomes the same player here, and the
	// dedicated-server slot is cleared because there is no longer one.
	setHostRef(gameFields, "PHST", hostRef)
	setHostRef(gameFields, "THST", hostRef)
	setHostRef(gameFields, "DHST", clearedHostRef())
	setHostRefList(gameFields, "EOBS", hostRef)
	// ADMN listed the dedicated host alongside the player; only the player is left.
	for i := range gameFields {
		if gameFields[i].Tag == "ADMN" {
			list, ok := gameFields[i].Value.(blaze.List)
			if !ok {
				continue
			}
			gameFields[i].Value = blaze.List{
				ElementType: list.ElementType,
				Values:      []any{host.identity.personaID},
			}
		}
	}
	setAddressIn(gameFields, "HNET", hostAddress, host.externalPort)
	setAddressIn(gameFields, "DNET", hostAddress, host.externalPort)

	roster := make([]any, 0, len(players))
	for _, player := range players {
		roster = append(roster, rosterEntryFor(rosterTemplate, game, player))
	}
	for i := range fields {
		if fields[i].Tag == "PROS" {
			list := fields[i].Value.(blaze.List)
			fields[i].Value = blaze.List{ElementType: list.ElementType, Values: roster}
		}
	}

	payload, err := blaze.Encode(fields)
	if err != nil {
		return blaze.Frame{}, fmt.Errorf("encode NotifyGameSetup: %w", err)
	}
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: host.identity.blazeID},
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
		Payload:  payload,
	}, nil
}
