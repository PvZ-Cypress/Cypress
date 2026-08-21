package cfb27blaze

import (
	_ "embed"
	"fmt"
	"sync"

	"cypress-servers/internal/blaze"
)

/*
Build NotifyPlayerJoining (4/21) from the captured frame as a template.

This notification introduces a joining player to everyone already in the game,
and it is what the client uses to build its picture of that player — identity,
slot, and the address a peer will dial.

It used to be hand-assembled from the handful of fields that looked necessary:
about 273 bytes against the 395 EA sends, missing CNTY, CONG, CSID, DSUI, ENCR,
JFPS, JVMM, LOC, NASP, PSET, RCRE, ROLE, SCEN, STAT, TIDX, UGID and UUID. A
client reading a frame that short goes looking for fields that are not there,
which is the same way the replayed NotifyGameSetup used to crash it.

So the captured frame supplies the shape and every field we do not understand,
and only the identity- and address-bearing fields are replaced:

	GID                          the game
	PDAT/PID, EXID, NAME         who is joining
	PDAT/AIDS/EAID/{NAME,NID,PCID}
	PDAT/SLOT, SID, TIDX         their seat
	PDAT/UID, CONG               their session
	PDAT/PNET/VALU/{EXIP,INIP}/IP the address a peer dials

Captured 2026-08-08 (proto_20260808_231405_PlayFriend_My_invite), 4/21.
*/

//go:embed fixtures/gm-21-notify-real.bin
var playerJoiningTemplatePayload []byte

var (
	playerJoiningOnce     sync.Once
	playerJoiningTemplate []blaze.Field
	playerJoiningErr      error
)

func loadPlayerJoiningTemplate() ([]blaze.Field, error) {
	playerJoiningOnce.Do(func() {
		fields, err := blaze.Decode(playerJoiningTemplatePayload)
		if err != nil {
			playerJoiningErr = fmt.Errorf("decode captured NotifyPlayerJoining: %w", err)
			return
		}
		playerJoiningTemplate = fields
	})
	return playerJoiningTemplate, playerJoiningErr
}

// buildPlayerJoiningNotification describes a player joining a game.
func buildPlayerJoiningNotification(gameID int64, player *gamePlayer) (blaze.Frame, error) {
	template, err := loadPlayerJoiningTemplate()
	if err != nil {
		return blaze.Frame{}, err
	}
	fields := cloneFields(template)
	identity := player.identity
	address := int64(player.externalIP)

	setScalar(fields, "GID", gameID)

	data, ok := childFields(fields, "PDAT")
	if !ok {
		return blaze.Frame{}, fmt.Errorf("captured NotifyPlayerJoining has no PDAT")
	}
	setScalar(data, "GID", gameID)
	setScalar(data, "PID", identity.personaID)
	// EXID is the Nucleus account id; it must match AIDS/EAID/NID.
	setScalar(data, "EXID", identity.nucleusID)
	setScalar(data, "NAME", identity.personaName)
	setScalar(data, "SID", player.slot+1)
	setScalar(data, "CSID", player.slot+1)
	setScalar(data, "TIDX", player.slot)
	setScalar(data, "UID", player.userSessionID)
	setScalar(data, "UUID", playerUUID(player))
	// The joiner's mesh identity, which is what the other client will connect to.
	// It has to be the group the joiner chose for itself and has to agree with the
	// PROS entry in NotifyGameSetup — the two used to contradict each other.
	setScalar(data, "CONG", player.connectionGroup)
	setScalar(data, "UGID", blaze.ObjectID{
		Type: blaze.ObjectType{Component: ComponentUserSessions, Type: 2},
		ID:   player.connectionGroup,
	})

	setAtPath(data, identity.personaName, "AIDS", "EAID", "NAME")
	setAtPath(data, identity.nucleusID, "AIDS", "EAID", "NID")
	setAtPath(data, identity.personaID, "AIDS", "EAID", "PCID")

	// The address a peer dials: the address this client reached us on, at the game
	// port it told us it listens on. On a VPN both halves are the same address.
	for _, half := range []string{"EXIP", "INIP"} {
		setAtPath(data, address, "PNET", "VALU", half, "IP")
		if player.externalPort > 0 {
			setAtPath(data, player.externalPort, "PNET", "VALU", half, "PORT")
		}
	}

	payload, err := blaze.Encode(fields)
	if err != nil {
		return blaze.Frame{}, fmt.Errorf("encode NotifyPlayerJoining: %w", err)
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentGameManager,
			Command:     NotifyGameManagerPlayerJoining,
			MessageType: blaze.MessageTypeNotification,
		},
		Payload: payload,
	}, nil
}
