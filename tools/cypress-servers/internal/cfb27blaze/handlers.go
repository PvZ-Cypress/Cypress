package cfb27blaze

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cypress-servers/internal/blaze"
)

const (
	ComponentAuthentication           uint16 = 0x0001
	CommandAuthenticationLogin        uint16 = 0x000a
	CommandAuthenticationLogout       uint16 = 0x0046
	CommandAuthenticationEntitlements uint16 = 0x001d
	CommandAuthenticationAccount      uint16 = 0x001e
	CommandAuthenticationEmailOptIn   uint16 = 0x00f2
	ComponentUtil                     uint16 = 0x0009
	ComponentGameManager              uint16 = 0x0004
	ComponentUserSessions             uint16 = 0x7802
	ComponentCensus                   uint16 = 0x000a
	ComponentOSDK                     uint16 = 2056
	ComponentGameReporting            uint16 = 2050
	ComponentPlayerConfig             uint16 = 2063
	ComponentBootStatus               uint16 = 2098
	ComponentAvatar                   uint16 = 2124
	ComponentSettings                 uint16 = 2249
	ComponentContent                  uint16 = 2360
	CommandUtilFetchClientConfig      uint16 = 0x0001
	CommandUtilPing                   uint16 = 0x0002
	CommandUtilPreAuth                uint16 = 0x0007
	CommandUtilPostAuth               uint16 = 0x0008
	CommandUtilSetClientMetrics       uint16 = 0x0016
	CommandUtilLoadUserSettings       uint16 = 0x000a
	CommandUtilSaveUserSettings       uint16 = 0x000b
	CommandUtilGetUserSettings        uint16 = 0x000c
	CommandUtilSetConnectionMode      uint16 = 0x001c
	CommandGameManagerCreateOrJoin    uint16 = 0x000a
	CommandGameReportingMatchContext  uint16 = 710
	CommandGameReportingBotContext    uint16 = 745
	CommandOSDKDynastyNotification    uint16 = 13
	CommandDynastyCreate              uint16 = 101
	CommandUserSessionsUserAdded      uint16 = 0x0008
	CommandUserSessionsData           uint16 = 0x0002
	ErrorCommandNotFound              uint16 = 0x0001
	ErrorSystem                       uint16 = 0x0002
	LocalBlazeID                      int64  = 1001
	localNucleusAccountID             int64  = 1001
	localPersonaID                    int64  = 1001
	localPlatform                     int64  = 4
	localLocale                       int64  = 1701729619
	localCountry                      int64  = 21843
	localSessionKey                          = "cypress-local-session-key"
)

type handler func(context.Context, blaze.Frame) ([]blaze.Field, uint16)

func (s *Service) registerDefaultHandlers() {
	s.handlers[route{ComponentAuthentication, CommandAuthenticationLogin}] = s.handleLocalLogin
	s.handlers[route{ComponentAuthentication, CommandAuthenticationLogout}] = handleEmptySuccess
	s.handlers[route{ComponentAuthentication, CommandAuthenticationEntitlements}] = s.handleAuthenticationEntitlements
	s.handlers[route{ComponentAuthentication, CommandAuthenticationAccount}] = s.handleAuthenticationAccount
	s.handlers[route{ComponentAuthentication, CommandAuthenticationEmailOptIn}] = handleAuthenticationEmailOptIn
	s.handlers[route{ComponentUtil, CommandUtilPing}] = handleUtilPing
	s.handlers[route{ComponentUtil, CommandUtilPreAuth}] = handleUtilPreAuth
	s.handlers[route{ComponentUtil, CommandUtilPostAuth}] = s.handleUtilPostAuth
	s.handlers[route{ComponentUtil, CommandUtilSetClientMetrics}] = handleEmptySuccess
	s.handlers[route{ComponentUtil, CommandUtilLoadUserSettings}] = handleUtilLoadUserSettings
	s.handlers[route{ComponentUtil, CommandUtilSaveUserSettings}] = handleEmptySuccess
	s.handlers[route{ComponentUtil, CommandUtilGetUserSettings}] = handleUtilGetUserSettings
	s.handlers[route{ComponentUtil, CommandUtilSetConnectionMode}] = handleEmptySuccess
	s.handlers[route{ComponentGameManager, CommandGameManagerCreateOrJoin}] = handleGameManagerCreateOrJoin

	// These are the first title-specific calls made after authentication.  Their
	// wire shapes are taken from the successful clean-room capture; keeping them
	// explicit prevents the client from receiving a burst of command-not-found
	// errors while its front-end services are being initialized.
	s.handlers[route{ComponentOSDK, 3002}] = handleEmptySuccess
	s.handlers[route{ComponentOSDK, 52}] = handleOSDKTrialConfig
	s.handlers[route{ComponentOSDK, 21}] = handleOSDKTelemetryLimits
	s.handlers[route{ComponentOSDK, 22}] = handleOSDKHeartbeat
	s.handlers[route{ComponentOSDK, 12}] = handleOSDKFeatureStatus
	s.handlers[route{ComponentOSDK, CommandOSDKDynastyNotification}] = handleEmptySuccess
	s.handlers[route{ComponentOSDK, 61}] = handleOSDKGameOnline
	s.handlers[route{ComponentOSDK, 3001}] = handleOSDKLeaderboardBootstrap
	s.handlers[route{ComponentUserSessions, 20}] = handleEmptySuccess
	s.handlers[route{ComponentUserSessions, 8}] = handleEmptySuccess
	s.handlers[route{ComponentContent, 13}] = handleEmptySuccess
	s.handlers[route{ComponentBootStatus, 1}] = handleBootStatus
	s.handlers[route{ComponentGameReporting, 901}] = handleGameReportingBootstrap
	s.handlers[route{ComponentGameReporting, CommandGameReportingMatchContext}] = handleEmptySuccess
	s.handlers[route{ComponentGameReporting, CommandGameReportingBotContext}] = handleEmptySuccess
	s.handlers[route{ComponentCensus, 5}] = handleCensusSubscribe
	s.handlers[route{ComponentCensus, 2}] = handleEmptySuccess
	s.handlers[route{ComponentPlayerConfig, 14}] = handlePlayerConfigBootstrap
	s.handlers[route{ComponentPlayerConfig, 14003}] = handlePlayerConfigDynastyProgress
	s.handlers[route{ComponentAvatar, 122}] = handleAvatarState
	s.handlers[route{ComponentAvatar, 128}] = handleAvatarConfig
	s.handlers[route{ComponentAvatar, 1203}] = handleAvatarEnvironment
	s.handlers[route{ComponentAvatar, 11}] = handleDynastyEntitlementInventory
	s.handlers[route{ComponentAvatar, 19}] = handleDynastyEntitlementBalances
	s.handlers[route{ComponentBootStatus, 2}] = handleEmptySuccess
	// Load/Continue Dynasty menu: 103 lists dynasties (FLST), 105 loads one.
	// Wire shapes from the 2026-08-08 EA capture; before this both returned
	// COMMAND_NOT_FOUND, leaving the menu empty and flickering.
	s.handlers[route{ComponentBootStatus, 103}] = s.handleDynastyList
	s.handlers[route{ComponentBootStatus, 105}] = s.handleDynastyLoad
	// Dynasty membership companions. In the capture, 582/592 return an empty
	// successful reply (they are the ack half of the 581/591 member/request
	// forms). Registering them stops COMMAND_NOT_FOUND when the members UI opens.
	// 581/591 themselves return large compact-database forms and are not yet
	// implemented; they need a two-account join capture to build faithfully.
	s.handlers[route{ComponentBootStatus, 582}] = handleEmptySuccess
	s.handlers[route{ComponentBootStatus, 592}] = handleEmptySuccess
	// Join lifecycle, wire shapes from the 2026-08-08 join capture:
	// 104 searches joinable dynasties (FLST), 111 applies to join, 164 leaves.
	s.handlers[route{ComponentBootStatus, 104}] = s.handleDynastyJoinSearch
	s.handlers[route{ComponentBootStatus, 111}] = s.handleDynastyJoinApply
	s.handlers[route{ComponentBootStatus, 164}] = s.handleDynastyLeave
	s.handlers[route{ComponentBootStatus, CommandDynastyCreate}] = s.handleDynastyCreate
	s.handlers[route{ComponentBootStatus, 162}] = s.handleDynastyClose
	s.handlers[route{ComponentBootStatus, 302}] = handleEmptySuccess
	s.handlers[route{ComponentBootStatus, 534}] = s.handleDynastyCoachContract
	s.handlers[route{ComponentBootStatus, 194}] = s.handleDynastyAssistantCoachConfirmation
	s.handlers[route{ComponentBootStatus, 1420}] = handleDynastyTimelineChoice
	// 164 is handled explicitly as Dynasty leave/cancel above; keep it out of
	// this blanket mutation-success list or the loop would overwrite it.
	for _, command := range []uint16{163, 303, 304, 305, 306, 307, 309, 414} {
		s.handlers[route{ComponentBootStatus, command}] = handleDynastyMutationSuccess
	}
	s.handlers[route{ComponentBootStatus, 107}] = s.handleDynastyAdvance
	for _, command := range []uint16{
		176, 192, 222, 272, 276, 312, 322, 362, 392, 412,
		502, 532, 542, 562, 801, 1132, 1152, 1252, 1272, 1411,
	} {
		s.handlers[route{ComponentBootStatus, command}] = handleEmptySuccess
	}
	s.handlers[route{ComponentBootStatus, 1262}] = handleEmptySuccess
	s.handlers[route{ComponentBootStatus, 1263}] = handleDynastyRefreshForm
	s.handlers[route{ComponentBootStatus, 1269}] = handleDynastyTeamBuilderEntry
	s.handlers[route{ComponentBootStatus, 1112}] = handleEmptySuccess
	s.handlers[route{ComponentSettings, 1}] = handleSettingsDefinitions
	s.handlers[route{ComponentSettings, 2}] = handleSettingsGroups

	// Empty-success routes observed in the successful Mascot Mashup ProtoSSL
	// capture (protossl_dump_1784743043.acp). These are deliberately limited to
	// capture-backed calls; do not turn unknown commands into blanket success.
	for _, captured := range []route{
		{ComponentOSDK, 1010}, {ComponentOSDK, 3101},
		{ComponentGameReporting, 7}, {ComponentGameReporting, 80},
		{ComponentGameReporting, 112}, {ComponentGameReporting, 113},
		{ComponentGameReporting, 504}, {ComponentGameReporting, 955},
		{ComponentPlayerConfig, 8},
		{ComponentBootStatus, 110},
		{ComponentContent, 1}, {ComponentContent, 2}, {ComponentContent, 3},
		{ComponentContent, 4}, {ComponentContent, 6}, {ComponentContent, 10},
		{2182, 2}, {2182, 5}, {2182, 8},
		{3076, 5},
		{ComponentUserSessions, 61}, {ComponentUserSessions, 64},
	} {
		s.handlers[captured] = handleEmptySuccess
	}

}

func handleEmptySuccess(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return nil, 0
}

func handlePlayerConfigDynastyProgress(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "FCGR", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "IMXF", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "MXIN", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "PAMT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "PPNT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "PTYP", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "STUS", Type: blaze.TypeInteger, Value: int64(0)},
		}},
	}, 0
}

func handleDynastyEntitlementInventory(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	attributes := func(key, value string) []blaze.Field {
		return []blaze.Field{
			{Tag: "DTYP", Type: blaze.TypeInteger, Value: int64(3)},
			{Tag: "KEY", Type: blaze.TypeString, Value: key},
			{Tag: "VAL", Type: blaze.TypeString, Value: value},
		}
	}
	staticAttribute := func(key, value string) []blaze.Field {
		fields := attributes(key, value)
		fields[0].Value = int64(2)
		return fields
	}
	item := []blaze.Field{
		{Tag: "CONS", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CRAT", Type: blaze.TypeString, Value: "2026-07-10T03:35:30.162Z"},
		{Tag: "CTNR", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "DYMC", Type: blaze.TypeList, Value: blaze.List{ElementType: blaze.TypeStruct, Values: []any{
			attributes("ItemConsumed", "false"), attributes("Consumed", "false"),
		}}},
		{Tag: "ICNT", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "IDEF", Type: blaze.TypeString, Value: "PreOrderStandard"},
		{Tag: "IGBL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "IMDF", Type: blaze.TypeString, Value: "GameEditionInfoMetaDef"},
		{Tag: "IREV", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "ITID", Type: blaze.TypeString, Value: "33c58a81-0dd7-4a5b-9965-6f436373c216"},
		{Tag: "QTLT", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "STCK", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "STTC", Type: blaze.TypeList, Value: blaze.List{ElementType: blaze.TypeStruct, Values: []any{
			staticAttribute("EditionName", "STANDARD"), staticAttribute("Description", "Pre Order Standard Edition of the Game"),
		}}},
		{Tag: "UPAT", Type: blaze.TypeString, Value: "2026-07-10T03:35:30.162Z"},
	}
	return []blaze.Field{
		{Tag: "INDL", Type: blaze.TypeList, Value: blaze.List{ElementType: blaze.TypeStruct, Values: []any{item}}},
		{Tag: "PAGE", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "LIMT", Type: blaze.TypeInteger, Value: int64(20)},
			{Tag: "OFFT", Type: blaze.TypeInteger, Value: int64(0)},
		}},
		{Tag: "TCNT", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func handleDynastyEntitlementBalances(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	balance := func(current, reserved, currency int64) []blaze.Field {
		return []blaze.Field{
			{Tag: "BALC", Type: blaze.TypeInteger, Value: current},
			{Tag: "BALR", Type: blaze.TypeInteger, Value: reserved},
			{Tag: "CRCY", Type: blaze.TypeInteger, Value: currency},
		}
	}
	return []blaze.Field{{Tag: "ENTR", Type: blaze.TypeList, Value: blaze.List{
		ElementType: blaze.TypeStruct,
		Values:      []any{balance(510, 3, 0), balance(0, 0, 1)},
	}}}, 0
}

func handleDynastyMutationSuccess(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func (s *Service) handleDynastyAdvance(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	if !s.config.PersistDynasty {
		return handleDynastyMutationSuccess(ctx, request)
	}
	cs := s.clientSessionFrom(ctx)
	sessionID := cs.activeDynastySession.Load()
	if sessionID == 0 {
		session, err := s.dynasty.EnsureSeeded(ctx, "Local Dynasty")
		if err != nil {
			return nil, ErrorSystem
		}
		sessionID = session.ID
		cs.activeDynastySession.Store(sessionID)
	}
	requestID := fmt.Sprintf("%s:%d", s.config.RunID, request.Header.MessageID)
	if s.config.RunID == "" {
		requestID = fmt.Sprintf("blaze:%d", request.Header.MessageID)
	}
	if _, err := s.dynasty.AdvanceSession(ctx, sessionID, 0, requestID); err != nil {
		return nil, ErrorSystem
	}
	return handleDynastyMutationSuccess(ctx, request)
}

func handleDynastyTeamBuilderEntry(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "FORM", Type: blaze.TypeStruct, Value: emptyDynastyDatabase(0)},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func handleDynastyRefreshForm(_ context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	values := map[string]int64{}
	for _, field := range fields {
		value, ok := field.Value.(int64)
		if ok {
			values[field.Tag] = value
		}
	}
	formKey, ok := values["FORM"]
	if !ok {
		return nil, ErrorCommandNotFound
	}
	return []blaze.Field{
		{Tag: "DATA", Type: blaze.TypeStruct, Value: emptyDynastyDatabase(formKey)},
		{Tag: "FORM", Type: blaze.TypeInteger, Value: formKey},
		{Tag: "PFIL", Type: blaze.TypeInteger, Value: values["PFIL"]},
		{Tag: "RDON", Type: blaze.TypeInteger, Value: values["RDON"]},
		{Tag: "SFIL", Type: blaze.TypeInteger, Value: values["SFIL"]},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func (s *Service) handleDynastyCoachContract(_ context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	values := map[string]int64{}
	for _, field := range fields {
		if value, ok := field.Value.(int64); ok {
			values[field.Tag] = value
		}
	}
	characterKey, hasCharacter := values["CHAR"]
	teamKey, hasTeam := values["TEAM"]
	if !hasCharacter || !hasTeam {
		return nil, ErrorCommandNotFound
	}
	teamName := "Selected Team"
	teamLogo := int64(1)
	coachName := "Local Coach"
	if identity, ok := dynastyTeamIdentityForKey(teamKey); ok {
		teamName = identity.teamName
		coachName = strings.TrimSpace(identity.coachFirstName + " " + identity.coachLastName)
	}
	if coach, ok := s.authoritativeHeadCoach(teamKey); ok {
		coachName = strings.TrimSpace(coach.FirstName + " " + coach.LastName)
	}
	if team, ok := s.authoritativeTeam(teamKey); ok {
		teamLogo = team.Logo
		teamName = team.LongName
		if teamName == "" {
			teamName = team.DisplayName
		}
	}
	return []blaze.Field{
		{Tag: "CHAR", Type: blaze.TypeInteger, Value: characterKey},
		{Tag: "CHNM", Type: blaze.TypeString, Value: coachName},
		{Tag: "ISCM", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TMKY", Type: blaze.TypeInteger, Value: teamKey},
		{Tag: "TMLD", Type: blaze.TypeInteger, Value: teamLogo},
		{Tag: "TMNM", Type: blaze.TypeString, Value: teamName},
		{Tag: "TMUR", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "UPOS", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func (s *Service) handleDynastyAssistantCoachConfirmation(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	var coachKey int64
	found := false
	for _, field := range fields {
		if field.Tag == "CKEY" {
			coachKey, found = field.Value.(int64)
			break
		}
	}
	if !found {
		return nil, ErrorCommandNotFound
	}
	teamLogo := int64(1)
	teamName := "Selected Team"
	if team, ok := s.authoritativeTeam(cs.selectedTeam.Load()); ok {
		teamLogo = team.Logo
		teamName = team.LongName
		if teamName == "" {
			teamName = team.DisplayName
		}
	}
	return []blaze.Field{
		{Tag: "CHNM", Type: blaze.TypeString, Value: ""},
		{Tag: "CKEY", Type: blaze.TypeInteger, Value: coachKey},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TMLD", Type: blaze.TypeInteger, Value: teamLogo},
		{Tag: "TMNM", Type: blaze.TypeString, Value: teamName},
		{Tag: "UDMD", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func handleDynastyTimelineChoice(_ context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	values := map[string]int64{}
	for _, field := range fields {
		if value, ok := field.Value.(int64); ok {
			values[field.Tag] = value
		}
	}
	for _, required := range []string{"CHID", "EVKY", "IFSE", "STKY"} {
		if _, ok := values[required]; !ok {
			return nil, ErrorCommandNotFound
		}
	}
	return []blaze.Field{
		{Tag: "CHDA", Type: blaze.TypeString, Value: ""},
		{Tag: "CHID", Type: blaze.TypeInteger, Value: values["CHID"]},
		{Tag: "EVKY", Type: blaze.TypeInteger, Value: values["EVKY"]},
		{Tag: "IFSE", Type: blaze.TypeInteger, Value: values["IFSE"]},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "STKY", Type: blaze.TypeInteger, Value: values["STKY"]},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TLCH", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func emptyDynastyDatabase(root int64) []blaze.Field {
	emptyDatabaseMap := func() blaze.Map {
		return blaze.Map{
			KeyType:   blaze.TypeInteger,
			ValueType: blaze.TypeStruct,
			Entries:   nil,
		}
	}
	return []blaze.Field{
		{Tag: "DICT", Type: blaze.TypeMap, Value: emptyDatabaseMap()},
		{Tag: "RIBC", Type: blaze.TypeInteger, Value: int64(17)},
		{Tag: "ROOT", Type: blaze.TypeInteger, Value: root},
		{Tag: "SIBC", Type: blaze.TypeInteger, Value: int64(14)},
		{Tag: "TABL", Type: blaze.TypeMap, Value: emptyDatabaseMap()},
	}
}

func handleGameManagerCreateOrJoin(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	// CFB27 issues this request when Continue is selected for a cloud Dynasty.
	// The client needs a concrete game identifier before it can advance to the
	// roster/setup flow; returning COMMAND_NOT_FOUND sends it back to the menu.
	return []blaze.Field{
		{Tag: "GID", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func (s *Service) handleDynastyClose(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	cs.dynastyContract.Store(0)
	cs.dynastyHub.Store(0)
	cs.dynastyAdvance.Store(0)
	cs.activeDynastySession.Store(0)
	cs.selectedTeam.Store(0)
	return nil, 0
}

func (s *Service) handleDynastyCreate(ctx context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	cs := s.clientSessionFrom(ctx)
	name := "Local Dynasty"
	requestFields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorSystem
	}
	for _, field := range requestFields {
		if field.Tag == "NAME" {
			if requestedName, ok := field.Value.(string); ok && requestedName != "" {
				name = requestedName
			}
			break
		}
	}

	session, err := s.dynasty.CreateSession(ctx, DynastySession{
		Name:        name,
		DynastyMode: "Online Dynasty",
		CurrentWeek: 1,
		Stage:       "preseason",
		MaxUsers:    32,
	})
	if err != nil {
		return nil, ErrorSystem
	}
	cs.dynastyContract.Store(0)
	cs.dynastyHub.Store(0)
	cs.dynastyAdvance.Store(0)
	cs.activeDynastySession.Store(session.ID)
	cs.selectedTeam.Store(0)
	version := func() []blaze.Field {
		return []blaze.Field{
			{Tag: "DMAJ", Type: blaze.TypeInteger, Value: int64(814)},
			{Tag: "DMIN", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "PDRN", Type: blaze.TypeInteger, Value: int64(3)},
		}
	}
	return []blaze.Field{
		{Tag: "CURR", Type: blaze.TypeStruct, Value: version()},
		{Tag: "LGID", Type: blaze.TypeInteger, Value: session.ID},
		{Tag: "ORIG", Type: blaze.TypeStruct, Value: version()},
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func handleUtilLoadUserSettings(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "DATA", Type: blaze.TypeString, Value: ""},
		{Tag: "KEY", Type: blaze.TypeString, Value: ""},
	}, 0
}

func handleUtilGetUserSettings(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "SMAP", Type: blaze.TypeMap, Value: blaze.Map{
		KeyType: blaze.TypeString, ValueType: blaze.TypeString,
		Entries: []blaze.MapEntry{{Key: "FirstTimeFlag", Value: "0"}},
	}}}, 0
}

func (s *Service) handleAuthenticationEntitlements(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	id := s.clientSessionFrom(ctx).identity
	entitlement := []blaze.Field{
		{Tag: "DEVI", Type: blaze.TypeString, Value: ""},
		{Tag: "GDAY", Type: blaze.TypeString, Value: "2026-01-01T00:00Z"},
		{Tag: "GNAM", Type: blaze.TypeString, Value: "CFBPC"},
		{Tag: "ID", Type: blaze.TypeInteger, Value: id.blazeID},
		{Tag: "ISCO", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PID", Type: blaze.TypeInteger, Value: id.personaID},
		{Tag: "PJID", Type: blaze.TypeString, Value: "324779"},
		{Tag: "PRCA", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "STAT", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "STRC", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TAG", Type: blaze.TypeString, Value: "ONLINE_ACCESS"},
		{Tag: "TDAY", Type: blaze.TypeString, Value: ""},
		{Tag: "UCNT", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "VERS", Type: blaze.TypeInteger, Value: int64(1)},
	}
	return []blaze.Field{{Tag: "NLST", Type: blaze.TypeList, Value: blaze.List{
		ElementType: blaze.TypeStruct,
		Values:      []any{entitlement},
	}}}, 0
}

func handleAuthenticationEmailOptIn(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "EAMC", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PMC", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func (s *Service) handleAuthenticationAccount(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	id := s.clientSessionFrom(ctx).identity
	return []blaze.Field{
		{Tag: "AMU", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "ASRC", Type: blaze.TypeString, Value: ""},
		{Tag: "CO", Type: blaze.TypeString, Value: "US"},
		{Tag: "DTCR", Type: blaze.TypeString, Value: "2026-01-01T00:00Z"},
		{Tag: "GOPT", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "LATH", Type: blaze.TypeString, Value: time.Now().UTC().Format("2006-01-02T15:04Z")},
		{Tag: "LN", Type: blaze.TypeString, Value: "en"},
		{Tag: "LOC", Type: blaze.TypeString, Value: ""},
		{Tag: "RC", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "STAS", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "STAT", Type: blaze.TypeInteger, Value: int64(2)},
		{Tag: "TPOT", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "UDU", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "UID", Type: blaze.TypeInteger, Value: id.nucleusID},
	}, 0
}

func handleOSDKTrialConfig(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "CNCL", Type: blaze.TypeString, Value: "Continue Trial"},
		{Tag: "PDID", Type: blaze.TypeString, Value: ""},
		{Tag: "TCMM", Type: blaze.TypeString, Value: "Offline play is enabled for this local server."},
		{Tag: "TCMT", Type: blaze.TypeString, Value: "Cypress Offline"},
		{Tag: "TEPT", Type: blaze.TypeString, Value: ""},
		{Tag: "TEXP", Type: blaze.TypeString, Value: ""},
		{Tag: "TREP", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "TRFR", Type: blaze.TypeInteger, Value: int64(60)},
		{Tag: "TTML", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "UPSL", Type: blaze.TypeString, Value: ""},
	}, 0
}

func handleOSDKTelemetryLimits(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "MTCL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "MTLU", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "MXCL", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "MXLU", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func handleOSDKHeartbeat(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "HBRP", Type: blaze.TypeInteger, Value: int64(0)}}, 0
}

func handleOSDKFeatureStatus(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "MESG", Type: blaze.TypeString, Value: ""},
		{Tag: "STAT", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TITL", Type: blaze.TypeString, Value: ""},
	}, 0
}

func handleOSDKGameOnline(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "GOIN", Type: blaze.TypeInteger, Value: int64(1)}}, 0
}

func handleOSDKLeaderboardBootstrap(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	emptyStructList := func() blaze.List {
		return blaze.List{ElementType: blaze.TypeStruct, Values: nil}
	}
	return []blaze.Field{
		{Tag: "LBCT", Type: blaze.TypeInteger, Value: int64(300)},
		{Tag: "LBVW", Type: blaze.TypeList, Value: emptyStructList()},
		{Tag: "PLBS", Type: blaze.TypeList, Value: emptyStructList()},
	}, 0
}

func handleBootStatus(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "SMSG", Type: blaze.TypeString, Value: ""},
		{Tag: "SUCC", Type: blaze.TypeInteger, Value: int64(1)},
	}, 0
}

func handleGameReportingBootstrap(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "MSG", Type: blaze.TypeStruct, Value: []blaze.Field{
		{Tag: "CACH", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "CAPT", Type: blaze.TypeString, Value: ""},
		{Tag: "CPMG", Type: blaze.TypeString, Value: ""},
		{Tag: "PUBK", Type: blaze.TypeString, Value: ""},
		{Tag: "RCDE", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "RMTL", Type: blaze.TypeString, Value: ""},
		{Tag: "RSTR", Type: blaze.TypeString, Value: ""},
		{Tag: "TIME", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TOUT", Type: blaze.TypeString, Value: ""},
		{Tag: "URLK", Type: blaze.TypeString, Value: ""},
	}}}, 0
}

func handleCensusSubscribe(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "CNP", Type: blaze.TypeInteger, Value: int64(600000000)},
		{Tag: "NTMT", Type: blaze.TypeInteger, Value: int64(3000000)},
		{Tag: "RTMT", Type: blaze.TypeInteger, Value: int64(2000000)},
	}, 0
}

func handlePlayerConfigBootstrap(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "FCSD", Type: blaze.TypeList, Value: blaze.List{
			ElementType: blaze.TypeInteger,
			Values:      []any{int64(400), int64(401), int64(402), int64(403), int64(404), int64(405)},
		}},
		{Tag: "HFVS", Type: blaze.TypeList, Value: blaze.List{ElementType: blaze.TypeStruct}},
		{Tag: "HLCC", Type: blaze.TypeMap, Value: blaze.Map{
			KeyType: blaze.TypeInteger, ValueType: blaze.TypeStruct,
		}},
		{Tag: "IAAE", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "MTXC", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "MUTQ", Type: blaze.TypeInteger, Value: int64(5)},
		{Tag: "PCCC", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "PLIL", Type: blaze.TypeList, Value: blaze.List{
			ElementType: blaze.TypeInteger,
			Values:      []any{int64(121), int64(122), int64(123), int64(124), int64(125)},
		}},
		{Tag: "PPCO", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "PYRD", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "REGF", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "SPWC", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "STCF", Type: blaze.TypeStruct, Value: []blaze.Field{}},
	}, 0
}

func handleAvatarState(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "ACFM", Type: blaze.TypeMap, Value: blaze.Map{
			KeyType: blaze.TypeString, ValueType: blaze.TypeMap,
		}},
		{Tag: "ATTR", Type: blaze.TypeList, Value: blaze.List{ElementType: blaze.TypeStruct}},
		{Tag: "GETN", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "IOMR", Type: blaze.TypeInteger, Value: int64(3)},
		{Tag: "IPNR", Type: blaze.TypeString, Value: ""},
		{Tag: "ISRU", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PUAI", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SCMR", Type: blaze.TypeInteger, Value: int64(5)},
	}, 0
}

func handleAvatarConfig(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "ADCT", Type: blaze.TypeString, Value: ""},
		{Tag: "ANSC", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "FMPA", Type: blaze.TypeStruct, Value: []blaze.Field{}},
		{Tag: "TPPC", Type: blaze.TypeStruct, Value: []blaze.Field{}},
	}, 0
}

func handleAvatarEnvironment(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "EACC", Type: blaze.TypeMap, Value: blaze.Map{
		KeyType:   blaze.TypeString,
		ValueType: blaze.TypeString,
		Entries: []blaze.MapEntry{
			{Key: "gamespaceId", Value: "cypress-local"},
		},
	}}}, 0
}

func handleSettingsDefinitions(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "LSIN", Type: blaze.TypeList, Value: blaze.List{
		ElementType: blaze.TypeStruct,
	}}}, 0
}

func handleSettingsGroups(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{{Tag: "LGRP", Type: blaze.TypeList, Value: blaze.List{
		ElementType: blaze.TypeStruct,
	}}}, 0
}

func handleUtilFetchClientConfig(_ context.Context, request blaze.Frame) ([]blaze.Field, uint16) {
	var entries []blaze.MapEntry
	requestFields, err := blaze.Decode(request.Payload)
	if err == nil {
		for _, field := range requestFields {
			configID, ok := field.Value.(string)
			if field.Tag != "CFID" || !ok || configID != "OSDK_MADSET" {
				continue
			}
			entries = []blaze.MapEntry{
				{Key: "CAREER_MAX_TEAMBUILDER_TEAMS", Value: "16"},
				{Key: "MCR_MAX_TEAMBUILDER_TEAMS", Value: "16"},
				{Key: "MCR_SID_LEAGUE_TEAMBUILDER", Value: "267"},
				{Key: "MCR_SID_RTG_TEAMBUILDER", Value: "287"},
			}
			break
		}
	}
	return []blaze.Field{
		{
			Tag:  "CONF",
			Type: blaze.TypeMap,
			Value: blaze.Map{
				KeyType:   blaze.TypeString,
				ValueType: blaze.TypeString,
				Entries:   entries,
			},
		},
	}, 0
}

func handleUtilPing(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	return []blaze.Field{
		{Tag: "STIM", Type: blaze.TypeInteger, Value: time.Now().Unix()},
	}, 0
}

func handleUtilPreAuth(_ context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	componentIDs := []any{
		int64(1), // Authentication
		int64(4), // GameManager
		int64(7), // Stats
		int64(8),
		int64(9), // Util
		int64(11),
		int64(12),
		int64(15), // Messaging
		int64(25), // AssociationLists
		int64(30720),
		int64(30722), // UserSessions
		int64(30723),
	}

	return []blaze.Field{
		{
			Tag:  "CIDS",
			Type: blaze.TypeList,
			Value: blaze.List{
				ElementType: blaze.TypeInteger,
				Values:      componentIDs,
			},
		},
		{
			Tag:  "CONF",
			Type: blaze.TypeStruct,
			Value: []blaze.Field{
				{
					Tag:  "CONF",
					Type: blaze.TypeMap,
					Value: blaze.Map{
						KeyType:   blaze.TypeString,
						ValueType: blaze.TypeString,
						Entries: []blaze.MapEntry{
							{Key: "pingPeriodInMs", Value: "30000"},
							{Key: "voipHeadsetUpdateRate", Value: "1000"},
						},
					},
				},
			},
		},
		{
			Tag:  "QOSS",
			Type: blaze.TypeStruct,
			Value: []blaze.Field{
				{
					Tag:  "BWPS",
					Type: blaze.TypeStruct,
					Value: []blaze.Field{
						{Tag: "PSA", Type: blaze.TypeString, Value: "127.0.0.1"},
						{Tag: "PSP", Type: blaze.TypeInteger, Value: int64(0)},
						{Tag: "SNA", Type: blaze.TypeString, Value: "local"},
					},
				},
				{Tag: "LNP", Type: blaze.TypeInteger, Value: int64(0)},
				{
					Tag:  "LTPS",
					Type: blaze.TypeMap,
					Value: blaze.Map{
						KeyType:   blaze.TypeString,
						ValueType: blaze.TypeStruct,
						Entries:   nil,
					},
				},
				{Tag: "SVID", Type: blaze.TypeInteger, Value: int64(0)},
			},
		},
		{Tag: "SVER", Type: blaze.TypeString, Value: "Cypress CFB27 Offline"},
	}, 0
}

func (s *Service) handleUtilPostAuth(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	id := s.clientSessionFrom(ctx).identity
	return []blaze.Field{
		{Tag: "TELE", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "ADRS", Type: blaze.TypeString, Value: "https://127.0.0.1:27920"},
			{Tag: "ANON", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "BKEY", Type: blaze.TypeBlob, Value: []byte(nil)},
			{Tag: "CTRY", Type: blaze.TypeInteger, Value: localCountry},
			{Tag: "DISA", Type: blaze.TypeString, Value: ""},
			{Tag: "ECCT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "EDCT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "FILT", Type: blaze.TypeString, Value: ""},
			{Tag: "LOC", Type: blaze.TypeInteger, Value: localLocale},
			{Tag: "MINR", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "NOOK", Type: blaze.TypeString, Value: ""},
			{Tag: "PENV", Type: blaze.TypeString, Value: "prod"},
			{Tag: "PORT", Type: blaze.TypeInteger, Value: int64(27920)},
			{Tag: "PURL", Type: blaze.TypeString, Value: "https://127.0.0.1:27920"},
			{Tag: "SDLY", Type: blaze.TypeInteger, Value: int64(15000)},
			{Tag: "SESS", Type: blaze.TypeString, Value: "cypress-local-telemetry"},
			{Tag: "SKEY", Type: blaze.TypeString, Value: localSessionKey},
			{Tag: "SPCT", Type: blaze.TypeInteger, Value: int64(75)},
			{Tag: "STIM", Type: blaze.TypeString, Value: "Default"},
			{Tag: "SVNM", Type: blaze.TypeString, Value: "cypress-local"},
		}},
		{Tag: "TICK", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "ADRS", Type: blaze.TypeString, Value: "127.0.0.1"},
			{Tag: "PORT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "SKEY", Type: blaze.TypeString, Value: ""},
		}},
		{Tag: "UROP", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "TMOP", Type: blaze.TypeInteger, Value: int64(1)},
			{Tag: "UID", Type: blaze.TypeInteger, Value: id.blazeID},
		}},
	}, 0
}

func (s *Service) handleLocalLogin(ctx context.Context, _ blaze.Frame) ([]blaze.Field, uint16) {
	id := s.clientSessionFrom(ctx).identity
	return []blaze.Field{
		{Tag: "ANON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "LQSZ", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "LQAR", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "LQPS", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "LQRA", Type: blaze.TypeInteger, Value: int64(20000000)},
			{Tag: "LQRT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "LQSZ", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "OQPS", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "OQSZ", Type: blaze.TypeInteger, Value: int64(0)},
		}},
		{Tag: "LQTK", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "SESS", Type: blaze.TypeStruct, Value: localSessionFields(s.config.Profile, id)},
		{Tag: "SPAM", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "UNDR", Type: blaze.TypeInteger, Value: int64(0)},
	}, 0
}

func localAccountIdentityFields(id clientIdentity) []blaze.Field {
	return []blaze.Field{
		{Tag: "EAID", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "NAME", Type: blaze.TypeString, Value: id.personaName},
			{Tag: "NID", Type: blaze.TypeInteger, Value: id.nucleusID},
			{Tag: "PCID", Type: blaze.TypeInteger, Value: id.personaID},
		}},
		{Tag: "EXID", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "PSID", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "STID", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "SWID", Type: blaze.TypeString, Value: ""},
			{Tag: "XBID", Type: blaze.TypeInteger, Value: int64(0)},
		}},
		{Tag: "PLAT", Type: blaze.TypeInteger, Value: localPlatform},
	}
}

func localSessionFields(profile string, id clientIdentity) []blaze.Field {
	return []blaze.Field{
		{Tag: "1CON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "AIDS", Type: blaze.TypeStruct, Value: localAccountIdentityFields(id)},
		{Tag: "BUID", Type: blaze.TypeInteger, Value: id.blazeID},
		{Tag: "FRST", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "GEO", Type: blaze.TypeInteger, Value: int64(1)},
		{Tag: "KEY", Type: blaze.TypeString, Value: id.sessionKey},
		{Tag: "LLOG", Type: blaze.TypeInteger, Value: time.Now().Unix()},
		{Tag: "PAAI", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PDTL", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "DSNM", Type: blaze.TypeString, Value: id.personaName},
			{Tag: "LAST", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "PID", Type: blaze.TypeInteger, Value: id.personaID},
			{Tag: "STAS", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "XREF", Type: blaze.TypeInteger, Value: id.nucleusID},
		}},
		{Tag: "UID", Type: blaze.TypeInteger, Value: id.nucleusID},
	}
}

func localUserAddedNotification(id clientIdentity) (blaze.Frame, error) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "1CON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "AIDS", Type: blaze.TypeStruct, Value: localAccountIdentityFields(id)},
		{Tag: "ALOC", Type: blaze.TypeInteger, Value: localLocale},
		{Tag: "ANON", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "APCS", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "SDAT", Type: blaze.TypeString, Value: ""},
		}},
		{Tag: "BUID", Type: blaze.TypeInteger, Value: id.blazeID},
		{Tag: "CGID", Type: blaze.TypeObjectID, Value: blaze.ObjectID{
			Type: blaze.ObjectType{Component: ComponentUserSessions, Type: 2},
			ID:   id.blazeID,
		}},
		{Tag: "CNTY", Type: blaze.TypeInteger, Value: localCountry},
		{Tag: "DSNM", Type: blaze.TypeString, Value: id.personaName},
		{Tag: "FRST", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "KEY", Type: blaze.TypeString, Value: id.sessionKey},
		{Tag: "LAST", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "LLOG", Type: blaze.TypeInteger, Value: time.Now().Unix()},
		{Tag: "NASP", Type: blaze.TypeString, Value: "cem_ea_id"},
		{Tag: "PAAI", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "PID", Type: blaze.TypeInteger, Value: id.personaID},
		{Tag: "PLAT", Type: blaze.TypeInteger, Value: localPlatform},
		{Tag: "UID", Type: blaze.TypeInteger, Value: id.nucleusID},
		{Tag: "USTP", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "XREF", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: id.blazeID},
		{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUserSessions,
			Command:     CommandUserSessionsUserAdded,
			MessageType: blaze.MessageTypeNotification,
			Options:     1,
		},
		Metadata: metadata,
		Payload:  payload,
	}, nil
}

func localUserSessionDataNotification(id clientIdentity) (blaze.Frame, error) {
	payload, err := blaze.Encode([]blaze.Field{
		{Tag: "APCS", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "SDAT", Type: blaze.TypeString, Value: ""},
		}},
		{Tag: "DATA", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "ADDR", Type: blaze.TypeUnion, Value: blaze.Union{ActiveMember: 0x7f}},
			{Tag: "BPS", Type: blaze.TypeString, Value: ""},
			{Tag: "CTY", Type: blaze.TypeString, Value: ""},
			{Tag: "CTYP", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "CVAR", Type: blaze.TypeVariable, Value: blaze.Variable{
				Marker: 0,
				Value: &blaze.Field{Tag: "DMAP", Type: blaze.TypeMap, Value: blaze.Map{
					KeyType:   blaze.TypeInteger,
					ValueType: blaze.TypeInteger,
					Entries: []blaze.MapEntry{
						{Key: int64(2013396993), Value: int64(0)},
					},
				}},
			}},
			{Tag: "HWFG", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "ISP", Type: blaze.TypeString, Value: ""},
			{Tag: "QDAT", Type: blaze.TypeStruct, Value: []blaze.Field{
				{Tag: "BWHR", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "CNFG", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "DBPS", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "NAHR", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "NATT", Type: blaze.TypeInteger, Value: int64(0)},
				{Tag: "UBPS", Type: blaze.TypeInteger, Value: int64(0)},
			}},
			{Tag: "TZ", Type: blaze.TypeString, Value: ""},
			{Tag: "UATT", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "ULST", Type: blaze.TypeList, Value: blaze.List{
				ElementType: blaze.TypeObjectID,
				Values: []any{blaze.ObjectID{
					Type: blaze.ObjectType{Component: ComponentUserSessions, Type: 2},
					ID:   id.blazeID,
				}},
			}},
			{Tag: "XPLT", Type: blaze.TypeInteger, Value: int64(1)},
		}},
		{Tag: "USER", Type: blaze.TypeStruct, Value: []blaze.Field{
			{Tag: "AID", Type: blaze.TypeInteger, Value: id.nucleusID},
			{Tag: "AIDS", Type: blaze.TypeStruct, Value: localAccountIdentityFields(id)},
			{Tag: "ALOC", Type: blaze.TypeInteger, Value: localLocale},
			{Tag: "CNTY", Type: blaze.TypeInteger, Value: localCountry},
			{Tag: "EXID", Type: blaze.TypeInteger, Value: int64(0)},
			{Tag: "ID", Type: blaze.TypeInteger, Value: id.personaID},
			{Tag: "ISPP", Type: blaze.TypeInteger, Value: int64(1)},
			{Tag: "NAME", Type: blaze.TypeString, Value: id.personaName},
			{Tag: "NASP", Type: blaze.TypeString, Value: "cem_ea_id"},
			{Tag: "ORIG", Type: blaze.TypeInteger, Value: id.personaID},
			{Tag: "PIDI", Type: blaze.TypeInteger, Value: id.nucleusID},
		}},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	metadata, err := blaze.Encode([]blaze.Field{
		{Tag: "CNTX", Type: blaze.TypeInteger, Value: id.blazeID},
		{Tag: "ERRC", Type: blaze.TypeInteger, Value: int64(0)},
	})
	if err != nil {
		return blaze.Frame{}, err
	}
	return blaze.Frame{
		Header: blaze.Header{
			Component:   ComponentUserSessions,
			Command:     CommandUserSessionsData,
			MessageType: blaze.MessageTypeNotification,
		},
		Metadata: metadata,
		Payload:  payload,
	}, nil
}
