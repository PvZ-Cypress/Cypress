package cfb27blaze

import (
	_ "embed"
)

/*
Stats / leaderboard configuration (component 28).

The client asks for this immediately after the game-setup notification, while
setting up an H2H match. Answering it with an error made the game abandon the
session and report "Unable to retrieve your progression information from EA
Servers", which reads like an account problem but is really this one reply
missing — pressing "Invite a Friend" was simply the moment it got asked.

The captured reply is column metadata for the H2H leaderboard (OPPONENT ID,
RESULT, SCORE and similar). It describes the game's own stat schema and carries
no player identity, so unlike NotifyGameSetup it can be replayed verbatim.

Captured 2026-08-08 (proto_20260808_233339_PF_Wadd_Invite_Wadd_DynInv), 28/7.
*/

const ComponentStats uint16 = 28

// CommandStatsGetStatGroup is the stat-group/leaderboard configuration request.
const CommandStatsGetStatGroup uint16 = 7

//go:embed fixtures/stats-28-7-reply.bin
var statsGroupConfigPayload []byte

func capturedStatsPayloads() map[route][]byte {
	return map[route][]byte{
		{ComponentStats, CommandStatsGetStatGroup}: statsGroupConfigPayload,
	}
}
