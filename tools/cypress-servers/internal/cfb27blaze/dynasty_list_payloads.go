package cfb27blaze

import (
	"context"
	"fmt"
	"strconv"

	"cypress-servers/internal/blaze"
)

// Wire shapes for the Load/Continue Dynasty menu, taken from a real EA capture
// (BootStatus 103 = list, 105 = load). Before this the server returned
// COMMAND_NOT_FOUND for both, so the menu rendered empty and flickered. 103
// returns FLST, a list of one struct per dynasty; 105 acknowledges a selection.
//
// Fields are emitted in ascending tag order because blaze.Encode preserves the
// given order and the TDF wire format requires tags sorted. Unknown-meaning
// fields are set to the constants observed in the capture so the client parses
// and renders the row; values derived from our own sessions are mapped through.

func (s *Service) handleDynastyList(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	sessions, err := s.dynasty.ListSessions(ctx)
	if err != nil {
		// An empty list is a valid "you have no dynasties" answer; a hard error
		// here would reproduce the empty-menu flicker we are fixing.
		return []blaze.Field{{Tag: "FLST", Type: blaze.TypeList, Value: blaze.List{
			ElementType: blaze.TypeStruct, Values: nil,
		}}}, 0
	}
	entries := make([]any, 0, len(sessions))
	for _, session := range sessions {
		entries = append(entries, s.dynastyListEntry(session))
	}
	return []blaze.Field{{Tag: "FLST", Type: blaze.TypeList, Value: blaze.List{
		ElementType: blaze.TypeStruct,
		Values:      entries,
	}}}, 0
}

func (s *Service) dynastyListEntry(session DynastySession) []blaze.Field {
	name := session.Name
	if name == "" {
		name = "CFB27 Dynasty"
	}

	// Team/coach identity for the row, when a team has been selected.
	var teamPresentation, teamLogo int64
	teamName := ""
	coachName := ""
	if team, ok := s.authoritativeTeam(session.SelectedTeamKey); ok {
		teamPresentation = team.PresentationID
		teamLogo = team.Logo
		teamName = team.LongName
		if teamName == "" {
			teamName = team.DisplayName
		}
	}
	if coach, ok := s.authoritativeHeadCoach(session.SelectedTeamKey); ok {
		coachName = trimName(coach.FirstName + " " + coach.LastName)
	}

	maxHumans := int64(session.MaxUsers)
	if maxHumans <= 0 {
		maxHumans = 32
	}

	settings := []blaze.Field{
		{Tag: "CLPE", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "CMEN", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CPEN", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "LGND", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "LTYP", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "MHUM", Type: blaze.TypeInteger, Value: maxHumans},
		{Tag: "PLCL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PUBL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "QTRL", Type: blaze.TypeInteger, Value: int64(5)},
		{Tag: "SKIL", Type: blaze.TypeInteger, Value: int64(3)},
		{Tag: "TYPE", Type: blaze.TypeInteger, Value: int64(0)},
	}

	commissioner := []blaze.Field{
		{Tag: "PERS", Type: blaze.TypeString, Value: s.config.Profile},
		{Tag: "USID", Type: blaze.TypeInteger, Value: localNucleusAccountID},
	}

	return []blaze.Field{
		{Tag: "ADVS", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "AVDA", Type: blaze.TypeString, Value: ""},
		{Tag: "CAYR", Type: blaze.TypeInteger, Value: int64(2026)},
		{Tag: "CHUM", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "CMPL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CMSH", Type: blaze.TypeStruct, Value: commissioner},
		{Tag: "CREA", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "FOFN", Type: blaze.TypeString, Value: coachName},
		{Tag: "FOFP", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "FOFT", Type: blaze.TypeInteger, Value: teamPresentation},
		{Tag: "ILID", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "IMPT", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "IUGC", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "JOIN", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: session.ID},
		{Tag: "NAME", Type: blaze.TypeString, Value: name},
		{Tag: "RSID", Type: blaze.TypeInteger, Value: int64(268435455)},
		{Tag: "SETT", Type: blaze.TypeStruct, Value: settings},
		{Tag: "SNSR", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SNTX", Type: blaze.TypeString, Value: dynastySeasonText(session)},
		{Tag: "SSLA", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TMLG", Type: blaze.TypeString, Value: strconv.FormatInt(teamLogo, 10)},
		{Tag: "TMLS", Type: blaze.TypeString, Value: strconv.FormatInt(teamPresentation, 10)},
		{Tag: "USPC", Type: blaze.TypeString, Value: ""},
		{Tag: "USTL", Type: blaze.TypeInteger, Value: teamLogo},
		{Tag: "USTN", Type: blaze.TypeString, Value: teamName},
	}
}

func (s *Service) handleDynastyLoad(_ context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	// Request carries the chosen LGID (0 when nothing is selected yet). The
	// captured reply is a bare acknowledgement; the actual league state loads
	// through the subsequent form/hub commands.
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	var leagueID int64
	for _, field := range fields {
		if field.Tag == "LGID" {
			if value, ok := field.Value.(int64); ok {
				leagueID = value
			}
		}
	}
	return []blaze.Field{
		{Tag: "ONLI", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "NOLG", Type: blaze.TypeInteger, Value: leagueID},
		}},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

// 104 = search for joinable dynasties. The request carries CRIT filters and an
// optional NAME; the reply is the same FLST list shape as 103. In this
// single-server model every session is joinable, so we return them all — the
// client filters client-side. (Wire shape from the 2026-08-08 join capture.)
func (s *Service) handleDynastyJoinSearch(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	return s.handleDynastyList(ctx, request)
}

// 111 = apply to join a league with a chosen team. Request:
// {ACIN:{INID,IUGC,PASS,TMID,TMNM,UCPE,UCPR}, LGID}. Reply is a bare SUCC ack;
// the league then loads through the normal form/hub commands.
//
// NOTE: the server still issues one identity to every connection, so this
// records intent but cannot yet distinguish two real players — that is the
// per-connection identity work in the multi-user design. Returning SUCC lets the
// client proceed into the league so the rest of the flow can be exercised.
func (s *Service) handleDynastyJoinApply(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	var leagueID, teamID int64
	for _, field := range fields {
		switch field.Tag {
		case "LGID":
			if value, ok := field.Value.(int64); ok {
				leagueID = value
			}
		case "ACIN":
			// Chosen team travels inside the ACIN sub-struct as TMID.
			if acin, ok := field.Value.([]blaze.Field); ok {
				for _, sub := range acin {
					if sub.Tag == "TMID" {
						if value, ok := sub.Value.(int64); ok {
							teamID = value
						}
					}
				}
			}
		}
	}
	if leagueID > 0 {
		cs := s.clientSessionFrom(ctx)
		cs.activeDynastySession.Store(leagueID)
		// Record the membership so the league tracks who has joined, under this
		// connection's own identity. Best-effort: a transient persistence failure
		// must not turn a valid join into a Blaze error that ejects the client.
		teamLabel := ""
		if teamID > 0 {
			teamLabel = strconv.FormatInt(teamID, 10)
		}
		_ = s.dynasty.RecordJoin(ctx, leagueID, cs.identity.personaName, teamLabel)
	}
	return []blaze.Field{
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

// 164 = leave a league / cancel a pending join request. Request:
// {LGID, RQID, SELR, SKEY}. Reply is a bare SUCC ack.
func (s *Service) handleDynastyLeave(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	var leagueID int64
	for _, field := range fields {
		if field.Tag == "LGID" {
			if value, ok := field.Value.(int64); ok {
				leagueID = value
			}
		}
	}
	if leagueID > 0 && cs.activeDynastySession.Load() == leagueID {
		cs.activeDynastySession.Store(0)
		cs.selectedTeam.Store(0)
	}
	return []blaze.Field{
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func dynastySeasonText(session DynastySession) string {
	week := session.CurrentWeek
	if week <= 0 {
		week = 1
	}
	if session.Stage == "preseason" || session.Stage == "" {
		return fmt.Sprintf("2026 PRE Wk %d", week)
	}
	return fmt.Sprintf("2026 REG Wk %d", week)
}

func trimName(name string) string {
	start := 0
	end := len(name)
	for start < end && name[start] == ' ' {
		start++
	}
	for end > start && name[end-1] == ' ' {
		end--
	}
	return name[start:end]
}
