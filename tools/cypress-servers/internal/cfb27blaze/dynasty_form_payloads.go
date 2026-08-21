package cfb27blaze

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27assets"
)

type rawRequestHandler func(context.Context, blaze.Frame) ([]byte, uint16)

//go:embed fixtures/dynasty-533-form-133300354-reply.bin
var dynastyForm133300354Payload []byte

//go:embed fixtures/dynasty-533-form-133300355-reply.bin
var dynastyForm133300355Payload []byte

//go:embed fixtures/dynasty-533-form-133300356-reply.bin
var dynastyForm133300356Payload []byte

//go:embed fixtures/dynasty-1111-cvtp-2-reply.bin
var dynastyCoachType2Payload []byte

//go:embed fixtures/dynasty-1111-cvtp-3-reply.bin
var dynastyCoachType3Payload []byte

const capturedDynastyTeamKey int64 = 830865409

func capturedDynastyFormHandlers(svc *Service) map[route]rawRequestHandler {
	return map[route]rawRequestHandler{
		{ComponentBootStatus, 533}:  svc.handleCapturedDynastyForm,
		{ComponentBootStatus, 1111}: svc.handleCapturedDynastyCoachSelection,
		// Client configuration carries the roster location; see client_config.go.
		{ComponentUtil, CommandUtilFetchClientConfig}: svc.handleUtilFetchClientConfigRaw,
	}
}

func (s *Service) handleCapturedDynastyForm(ctx context.Context, request blaze.Frame) ([]byte, uint16) {
	cs := s.clientSessionFrom(ctx)
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	for _, field := range fields {
		if field.Tag != "FORM" {
			continue
		}
		formID, ok := field.Value.(int64)
		if !ok {
			return nil, ErrorCommandNotFound
		}
		switch formID {
		case 133300354:
			s.rememberSelectedTeam(cs, fields, "PFIL")
			return s.selectedDynastyStaffPayload(cs, dynastyForm133300354Payload), 0
		case 133300355:
			s.rememberSelectedTeam(cs, fields, "PFIL")
			// This form is the complete team-selection database. It already
			// contains every team, so rewriting Akron into the selected team
			// creates a duplicate with Akron's styling and identity remnants.
			return append([]byte(nil), dynastyForm133300355Payload...), 0
		case 133300356:
			return dynastyForm133300356Payload, 0
		default:
			return nil, ErrorCommandNotFound
		}
	}
	return nil, ErrorCommandNotFound
}

func (s *Service) handleCapturedDynastyCoachSelection(ctx context.Context, request blaze.Frame) ([]byte, uint16) {
	cs := s.clientSessionFrom(ctx)
	fields, err := blaze.Decode(request.Payload)
	if err != nil {
		return nil, ErrorCommandNotFound
	}
	var coachKey, coachType, teamKey int64
	var hasCoachType, hasTeam bool
	for _, field := range fields {
		switch field.Tag {
		case "CHKY":
			coachKey, _ = field.Value.(int64)
		case "CVTP":
			coachType, hasCoachType = field.Value.(int64)
		case "TMKY":
			teamKey, hasTeam = field.Value.(int64)
		}
	}
	if !hasCoachType || (coachType != 2 && coachType != 3) {
		return nil, ErrorCommandNotFound
	}
	if hasTeam && teamKey > 0 && teamKey != 4294967295 {
		cs.selectedTeam.Store(teamKey)
		if s.config.PersistDynasty {
			sessionID := cs.activeDynastySession.Load()
			if sessionID == 0 {
				session, seedErr := s.dynasty.EnsureSeeded(ctx, "Local Dynasty")
				if seedErr != nil {
					return nil, ErrorSystem
				}
				sessionID = session.ID
				cs.activeDynastySession.Store(sessionID)
			}
			// The selected team is already retained in memory. A transient or
			// misconfigured persistence backend must not turn a valid coach
			// selection into a fatal Blaze error that ejects the client.
			_, _ = s.dynasty.SelectTeam(ctx, sessionID, teamKey, coachKey)
		}
	}
	if coachType == 2 {
		return s.selectedDynastyCoachPayload(cs, dynastyCoachType2Payload, coachKey), 0
	}
	return s.selectedDynastyCoachPayload(cs, dynastyCoachType3Payload, coachKey), 0
}

func (s *Service) rememberSelectedTeam(cs *clientSession, fields []blaze.Field, tag string) {
	for _, field := range fields {
		if field.Tag == tag {
			if value, ok := field.Value.(int64); ok && value > 0 && value != 4294967295 {
				cs.selectedTeam.Store(value)
			}
			return
		}
	}
}

func (s *Service) localizeSelectedTeam(cs *clientSession, payload []byte) []byte {
	teamKey := cs.selectedTeam.Load()
	if teamKey == 0 {
		return append([]byte(nil), payload...)
	}
	localized := append([]byte(nil), payload...)
	capturedIdentity, capturedOK := dynastyTeamIdentityForKey(capturedDynastyTeamKey)
	selectedIdentity, selectedOK := s.authoritativeTeamIdentity(teamKey)
	if !selectedOK {
		selectedIdentity, selectedOK = dynastyTeamIdentityForKey(teamKey)
	}
	if capturedOK && selectedOK {
		localized = replaceDynastyDatabaseString(localized, capturedIdentity.teamName, selectedIdentity.teamName)
		localized = replaceDynastyDatabaseString(localized, capturedIdentity.nickname, selectedIdentity.nickname)
		localized = replaceDynastyDatabaseStringWhenFollowed(
			localized,
			capturedIdentity.coachFirstName,
			selectedIdentity.coachFirstName,
			capturedIdentity.coachLastName,
			128)
		localized = replaceDynastyDatabaseString(localized, capturedIdentity.coachLastName, selectedIdentity.coachLastName)
		localized = replaceDynastyDatabaseString(localized, capturedIdentity.shortCoachName(), selectedIdentity.shortCoachName())
	}
	if capturedTeam, capturedTeamOK := s.authoritativeTeam(capturedDynastyTeamKey); capturedTeamOK {
		if selectedTeam, selectedTeamOK := s.authoritativeTeam(teamKey); selectedTeamOK {
			assetPairs := [][2]string{
				{capturedTeam.AssetName, selectedTeam.AssetName},
				{capturedTeam.ShortName, selectedTeam.ShortName},
				{capturedTeam.NicknameAlt, selectedTeam.NicknameAlt},
			}
			for _, key := range []string{
				"Hashtag1", "Hashtag2", "Mascot_AssetName", "TEAM_DBASSETNAME",
				"TEAM_PREFIX_NAME", "UniformAssetName",
			} {
				assetPairs = append(assetPairs, [2]string{
					teamDataString(capturedTeam, key), teamDataString(selectedTeam, key),
				})
			}
			for _, pair := range assetPairs {
				localized = replaceDynastyDatabaseString(localized, pair[0], pair[1])
			}
		}
	}
	from, err := encodedIntegerValue(capturedDynastyTeamKey)
	if err != nil {
		return localized
	}
	to, err := encodedIntegerValue(teamKey)
	if err != nil || len(from) != len(to) {
		return localized
	}
	localized = bytes.ReplaceAll(localized, from, to)
	team, ok := s.authoritativeTeam(teamKey)
	if !ok {
		return localized
	}
	coach, _ := s.authoritativeHeadCoach(teamKey)
	return overlayAuthoritativeTeam(localized, team, coach)
}

func teamDataString(team cfb27assets.Team, key string) string {
	value, ok := team.Data[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (s *Service) authoritativeTeamIdentity(teamKey int64) (dynastyTeamIdentity, bool) {
	team, ok := s.authoritativeTeam(teamKey)
	if !ok {
		return dynastyTeamIdentity{}, false
	}
	identity := dynastyTeamIdentity{
		teamName: team.LongName,
		nickname: team.Nickname,
	}
	if identity.teamName == "" {
		identity.teamName = team.DisplayName
	}
	if coach, found := s.authoritativeHeadCoach(teamKey); found {
		identity.coachFirstName = coach.FirstName
		identity.coachLastName = coach.LastName
	}
	return identity, identity.teamName != "" && identity.nickname != "" &&
		identity.coachFirstName != "" && identity.coachLastName != ""
}

func (s *Service) authoritativeTeam(teamKey int64) (cfb27assets.Team, bool) {
	if s.coachCatalog == nil || s.coachCatalogError != nil {
		return cfb27assets.Team{}, false
	}
	return s.coachCatalog.TeamByKey(teamKey)
}

func authoritativeTeamOverlayFields(team cfb27assets.Team, coach cfb27assets.Coach) []blaze.Field {
	fields := []blaze.Field{
		{Tag: "TMID", Type: blaze.TypeInteger, Value: team.PresentationID},
		{Tag: "TMLI", Type: blaze.TypeInteger, Value: team.Logo},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: team.Logo},
		{Tag: "TDEF", Type: blaze.TypeInteger, Value: team.DefensiveRating},
		{Tag: "TOFF", Type: blaze.TypeInteger, Value: team.OffensiveRating},
		{Tag: "DEFR", Type: blaze.TypeInteger, Value: team.DefensiveRating},
		{Tag: "OFFR", Type: blaze.TypeInteger, Value: team.OffensiveRating},
		{Tag: "OVLR", Type: blaze.TypeInteger, Value: team.OverallRating},
		{Tag: "DEFS", Type: blaze.TypeInteger, Value: team.DefensiveSchemeValue},
		{Tag: "OFFS", Type: blaze.TypeInteger, Value: team.OffensiveSchemeValue},
		{Tag: "CDSC", Type: blaze.TypeInteger, Value: team.DefensiveSchemeValue},
		{Tag: "COSC", Type: blaze.TypeInteger, Value: team.OffensiveSchemeValue},
		{Tag: "SCHP", Type: blaze.TypeInteger, Value: team.TeamPrestige},
		{Tag: "RANK", Type: blaze.TypeInteger, Value: team.PrestigeRank},
		{Tag: "TPCR", Type: blaze.TypeInteger, Value: team.PrimaryColor},
		{Tag: "TSCR", Type: blaze.TypeInteger, Value: team.SecondaryColor},
	}
	if team.Conference.Name != "" {
		fields = append(fields, blaze.Field{Tag: "TSCO", Type: blaze.TypeString, Value: team.Conference.Name})
	}
	if coach.FirstName != "" && coach.LastName != "" {
		if id, ok := authoritativeCoachID(coach); ok {
			fields = append(fields,
				blaze.Field{Tag: "USID", Type: blaze.TypeInteger, Value: dynastyCoachPortraitKeyBase + id},
				blaze.Field{Tag: "USOV", Type: blaze.TypeInteger, Value: coach.Level},
			)
		}
	}
	return fields
}

func (s *Service) localizeDynastyHub(cs *clientSession, payload []byte) []byte {
	teamKey := cs.selectedTeam.Load()
	team, ok := s.authoritativeTeam(teamKey)
	if teamKey == 0 || !ok {
		return s.localizeSelectedTeam(cs, payload)
	}
	capturedIdentity, ok := dynastyTeamIdentityForKey(capturedDynastyTeamKey)
	if !ok {
		return s.localizeSelectedTeam(cs, payload)
	}
	anchorWire, err := blaze.Encode([]blaze.Field{{Tag: "TLNM", Type: blaze.TypeString, Value: capturedIdentity.teamName}})
	if err != nil {
		return s.localizeSelectedTeam(cs, payload)
	}
	anchor := bytes.Index(payload, anchorWire)
	if anchor < 0 {
		return s.localizeSelectedTeam(cs, payload)
	}
	teamName := team.LongName
	if teamName == "" {
		teamName = team.DisplayName
	}
	coach, _ := s.authoritativeHeadCoach(teamKey)
	fields := append(authoritativeTeamOverlayFields(team, coach),
		blaze.Field{Tag: "FTNM", Type: blaze.TypeString, Value: teamName},
		blaze.Field{Tag: "TLNM", Type: blaze.TypeString, Value: teamName},
		blaze.Field{Tag: "TNME", Type: blaze.TypeString, Value: team.Nickname},
		blaze.Field{Tag: "TEAM", Type: blaze.TypeInteger, Value: team.TeamKey},
	)
	start := anchor - 256
	if start < 0 {
		start = 0
	}
	end := anchor + len(anchorWire) + 384
	if end > len(payload) {
		end = len(payload)
	}
	segment := replaceAllDynastyScalarFields(payload[start:end], fields)
	result := make([]byte, 0, len(payload)-(end-start)+len(segment))
	result = append(result, payload[:start]...)
	result = append(result, segment...)
	result = append(result, payload[end:]...)
	if id, found := authoritativeCoachID(coach); found && teamKey != capturedDynastyTeamKey {
		result = replaceDynastyScalarAfterLastStringAnchor(result, "TENA", capturedIdentity.teamName,
			blaze.Field{Tag: "USID", Type: blaze.TypeInteger, Value: int64(0)})
		result = replaceDynastyScalarAfterLastStringAnchor(result, "TENA", teamName,
			blaze.Field{Tag: "USID", Type: blaze.TypeInteger, Value: dynastyCoachPortraitKeyBase + id})
	}
	return result
}

func replaceDynastyScalarAfterLastStringAnchor(payload []byte, anchorTag, anchorValue string, field blaze.Field) []byte {
	anchor, err := blaze.Encode([]blaze.Field{{Tag: anchorTag, Type: blaze.TypeString, Value: anchorValue}})
	if err != nil {
		return payload
	}
	anchorOffset := bytes.LastIndex(payload, anchor)
	if anchorOffset < 0 {
		return payload
	}
	searchStart := anchorOffset + len(anchor)
	searchEnd := searchStart + 512
	if searchEnd > len(payload) {
		searchEnd = len(payload)
	}
	fieldTag, err := blaze.EncodeTag(field.Tag)
	if err != nil {
		return payload
	}
	relative := bytes.Index(payload[searchStart:searchEnd], fieldTag[:])
	if relative < 0 {
		return payload
	}
	fieldOffset := searchStart + relative
	_, fieldEnd, ok := dynastyScalarFieldBounds(payload[fieldOffset:], field.Tag)
	if !ok {
		return payload
	}
	replacement, err := blaze.Encode([]blaze.Field{field})
	if err != nil {
		return payload
	}
	result := make([]byte, 0, len(payload)-fieldEnd+len(replacement))
	result = append(result, payload[:fieldOffset]...)
	result = append(result, replacement...)
	result = append(result, payload[fieldOffset+fieldEnd:]...)
	return result
}

func overlayAuthoritativeTeam(payload []byte, team cfb27assets.Team, coach cfb27assets.Coach) []byte {
	fields := authoritativeTeamOverlayFields(team, coach)
	/* Coach-card and coach-detail records are patched separately. Their compact
	   database tags do not share meanings with the team summary tags. */

	result := append([]byte(nil), payload...)
	teamName := team.LongName
	if teamName == "" {
		teamName = team.DisplayName
	}
	anchors := []struct {
		field         blaze.Field
		before, after int
	}{
		{blaze.Field{Tag: "TLNM", Type: blaze.TypeString, Value: teamName}, 160, 384},
		{blaze.Field{Tag: "TENA", Type: blaze.TypeString, Value: teamName}, 160, 384},
		{blaze.Field{Tag: "TMNM", Type: blaze.TypeString, Value: teamName}, 384, 384},
		{blaze.Field{Tag: "TMKY", Type: blaze.TypeInteger, Value: team.TeamKey}, 384, 384},
		{blaze.Field{Tag: "TEAM", Type: blaze.TypeInteger, Value: team.TeamKey}, 384, 384},
	}
	for _, anchor := range anchors {
		result = replaceDynastyFieldsNearEveryAnchor(result, anchor.field, anchor.before, anchor.after, fields)
	}
	return result
}

func replaceDynastyFieldsNearEveryAnchor(payload []byte, anchor blaze.Field, before, after int, fields []blaze.Field) []byte {
	anchorWire, err := blaze.Encode([]blaze.Field{anchor})
	if err != nil || len(anchorWire) == 0 {
		return payload
	}
	positions := make([]int, 0, 4)
	for cursor := 0; cursor < len(payload); {
		relative := bytes.Index(payload[cursor:], anchorWire)
		if relative < 0 {
			break
		}
		position := cursor + relative
		positions = append(positions, position)
		cursor = position + len(anchorWire)
	}
	result := append([]byte(nil), payload...)
	for index := len(positions) - 1; index >= 0; index-- {
		start := positions[index] - before
		if start < 0 {
			start = 0
		}
		end := positions[index] + len(anchorWire) + after
		if end > len(result) {
			end = len(result)
		}
		segment := replaceAllDynastyScalarFields(result[start:end], fields)
		updated := make([]byte, 0, len(result)-(end-start)+len(segment))
		updated = append(updated, result[:start]...)
		updated = append(updated, segment...)
		updated = append(updated, result[end:]...)
		result = updated
	}
	return result
}

func replaceAllDynastyScalarFields(payload []byte, fields []blaze.Field) []byte {
	result := append([]byte(nil), payload...)
	for _, field := range fields {
		replacement, err := blaze.Encode([]blaze.Field{field})
		if err != nil {
			continue
		}
		encodedTag, err := blaze.EncodeTag(field.Tag)
		if err != nil {
			continue
		}
		for cursor := 0; cursor+5 <= len(result); {
			relative := bytes.Index(result[cursor:], encodedTag[:])
			if relative < 0 {
				break
			}
			start := cursor + relative
			fieldStart, fieldEnd, ok := dynastyScalarFieldBounds(result[start:], field.Tag)
			if !ok || fieldStart != 0 {
				cursor = start + 1
				continue
			}
			fieldEnd += start
			updated := make([]byte, 0, len(result)-(fieldEnd-start)+len(replacement))
			updated = append(updated, result[:start]...)
			updated = append(updated, replacement...)
			updated = append(updated, result[fieldEnd:]...)
			result = updated
			cursor = start + len(replacement)
		}
	}
	return result
}

type dynastyTeamIdentity struct {
	teamName       string
	nickname       string
	coachFirstName string
	coachLastName  string
}

func (identity dynastyTeamIdentity) shortCoachName() string {
	if identity.coachFirstName == "" || identity.coachLastName == "" {
		return ""
	}
	return identity.coachFirstName[:1] + ". " + identity.coachLastName
}

func dynastyTeamIdentityForKey(teamKey int64) (dynastyTeamIdentity, bool) {
	encodedKey, err := encodedIntegerValue(teamKey)
	if err != nil {
		return dynastyTeamIdentity{}, false
	}
	keyOffset := bytes.LastIndex(dynastyForm133300356Payload, encodedKey)
	if keyOffset < 0 {
		return dynastyTeamIdentity{}, false
	}
	mainTagOffset := bytes.LastIndex(dynastyForm133300356Payload[:keyOffset], dynastyTeamMainTag)
	if mainTagOffset < 1 {
		return dynastyTeamIdentity{}, false
	}
	mainOffset := mainTagOffset - 1
	identity := dynastyTeamIdentity{}
	identity.coachFirstName, _ = dynastyDatabaseStringAfterTag(dynastyForm133300356Payload, mainOffset, keyOffset, []byte{0x8e, 0x6b, 0xad, 0x01})
	identity.coachLastName, _ = dynastyDatabaseStringAfterTag(dynastyForm133300356Payload, mainOffset, keyOffset, []byte{0x8e, 0xcb, 0xad, 0x01})
	identity.teamName, _ = dynastyDatabaseStringAfterTag(dynastyForm133300356Payload, mainOffset, keyOffset, []byte{0xd2, 0xdb, 0xad, 0x01})
	identity.nickname, _ = dynastyDatabaseStringAfterTag(dynastyForm133300356Payload, keyOffset, len(dynastyForm133300356Payload), []byte{0xd2, 0xeb, 0xae, 0x01})
	return identity, identity.teamName != "" && identity.nickname != "" && identity.coachFirstName != "" && identity.coachLastName != ""
}

func dynastyDatabaseStringAfterTag(data []byte, start int, end int, tag []byte) (string, bool) {
	if start < 0 || end > len(data) || start >= end {
		return "", false
	}
	relative := bytes.Index(data[start:end], tag)
	if relative < 0 {
		return "", false
	}
	lengthOffset := start + relative + len(tag)
	if lengthOffset >= end {
		return "", false
	}
	length := int(data[lengthOffset])
	valueOffset := lengthOffset + 1
	if length < 1 || valueOffset+length > end || data[valueOffset+length-1] != 0 {
		return "", false
	}
	return string(data[valueOffset : valueOffset+length-1]), true
}

func replaceDynastyDatabaseString(payload []byte, from string, to string) []byte {
	if from == "" || to == "" || from == to || len(from) >= 63 || len(to) >= 63 {
		return payload
	}
	needle := dynastyDatabaseStringField(from)
	replacement := dynastyDatabaseStringField(to)
	return bytes.ReplaceAll(payload, needle, replacement)
}

func replaceDynastyDatabaseStringWhenFollowed(payload []byte, from string, to string, following string, maxDistance int) []byte {
	if from == "" || to == "" || following == "" || maxDistance < 1 || len(from) >= 63 || len(to) >= 63 || len(following) >= 63 {
		return payload
	}
	needle := dynastyDatabaseStringField(from)
	replacement := dynastyDatabaseStringField(to)
	followingNeedle := dynastyDatabaseStringField(following)
	result := make([]byte, 0, len(payload))
	cursor := 0
	for cursor < len(payload) {
		relative := bytes.Index(payload[cursor:], needle)
		if relative < 0 {
			result = append(result, payload[cursor:]...)
			break
		}
		offset := cursor + relative
		after := offset + len(needle)
		windowEnd := after + maxDistance
		if windowEnd > len(payload) {
			windowEnd = len(payload)
		}
		result = append(result, payload[cursor:offset]...)
		if bytes.Contains(payload[after:windowEnd], followingNeedle) {
			result = append(result, replacement...)
		} else {
			result = append(result, needle...)
		}
		cursor = after
	}
	return result
}

func dynastyDatabaseStringField(value string) []byte {
	field := make([]byte, 0, len(value)+2)
	field = append(field, byte(len(value)+1))
	field = append(field, value...)
	return append(field, 0)
}

var (
	dynastyCoachHistoryTag  = []byte{0xa2, 0x9c, 0xf4, 0x04}
	dynastyTeamMainTag      = []byte{0x8e, 0x38, 0xe5, 0x04}
	dynastyTeamHistoryTag   = []byte{0xa3, 0x3a, 0x66, 0x04}
	dynastyTeamAfterHistory = []byte{0x8e, 0x4c, 0xe3}
)

const dynastyCoachPortraitKeyBase int64 = 547356166

type dynastyHeadCoach struct {
	coachKey        int64
	portrait        int64
	firstName       string
	lastName        string
	archetype       int64
	defensiveScheme int64
	offensiveScheme int64
	level           int64
	teamID          int64
	pipeline        int64
}

func (s *Service) selectedDynastyStaffPayload(cs *clientSession, captured []byte) []byte {
	teamKey := cs.selectedTeam.Load()
	if teamKey == 0 || teamKey == capturedDynastyTeamKey {
		return append([]byte(nil), captured...)
	}
	team, ok := s.authoritativeTeam(teamKey)
	if !ok || s.coachCatalog == nil {
		return s.localizeSelectedTeam(cs, captured)
	}
	staff := s.coachCatalog.CoachesByTeamIndex(team.TeamIndex)
	if len(staff) < 3 {
		return s.localizeSelectedTeam(cs, captured)
	}
	return replaceAuthoritativeStaffCards(s.localizeSelectedTeam(cs, captured), team, staff)
}

func replaceAuthoritativeStaffCards(payload []byte, team cfb27assets.Team, staff []cfb27assets.Coach) []byte {
	byType := make(map[int64]cfb27assets.Coach, 3)
	for _, coach := range staff {
		switch strings.ToLower(strings.TrimSpace(coach.Position)) {
		case "headcoach":
			byType[2] = coach
		case "defensivecoordinator":
			byType[1] = coach
		case "offensivecoordinator":
			byType[0] = coach
		}
	}
	teamName := team.LongName
	if teamName == "" {
		teamName = team.DisplayName
	}
	anchorWire, err := blaze.Encode([]blaze.Field{{Tag: "TMNM", Type: blaze.TypeString, Value: teamName}})
	if err != nil {
		return payload
	}
	startTag, err := blaze.EncodeTag("CDSC")
	if err != nil {
		return payload
	}
	anchors := make([]int, 0, 3)
	for cursor := 0; cursor < len(payload); {
		relative := bytes.Index(payload[cursor:], anchorWire)
		if relative < 0 {
			break
		}
		position := cursor + relative
		anchors = append(anchors, position)
		cursor = position + len(anchorWire)
	}
	if len(anchors) != 3 {
		return payload
	}
	starts := make([]int, len(anchors))
	for index, position := range anchors {
		searchStart := position - 220
		if searchStart < 0 {
			searchStart = 0
		}
		relative := bytes.LastIndex(payload[searchStart:position], startTag[:])
		if relative < 0 {
			return payload
		}
		starts[index] = searchStart + relative
	}
	result := append([]byte(nil), payload...)
	for index := len(starts) - 1; index >= 0; index-- {
		start := starts[index]
		end := len(result)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		coachType, ok := dynastyIntegerScalar(result[start:end], "CHAT")
		if !ok {
			continue
		}
		coach, ok := byType[coachType]
		if !ok {
			continue
		}
		fields := []blaze.Field{
			{Tag: "FNAM", Type: blaze.TypeString, Value: coach.FirstName},
			{Tag: "LNAM", Type: blaze.TypeString, Value: coach.LastName},
			{Tag: "PPOR", Type: blaze.TypeInteger, Value: coach.Portrait},
			{Tag: "PERF", Type: blaze.TypeInteger, Value: coach.Level},
			{Tag: "PIPE", Type: blaze.TypeInteger, Value: coach.PipelineValue},
			{Tag: "CTAE", Type: blaze.TypeInteger, Value: coach.ArchetypeValue},
			{Tag: "CPRE", Type: blaze.TypeInteger, Value: coach.PrestigeValue},
			{Tag: "TEAM", Type: blaze.TypeInteger, Value: team.TeamKey},
			{Tag: "TMID", Type: blaze.TypeInteger, Value: team.PresentationID},
			{Tag: "TMLG", Type: blaze.TypeInteger, Value: team.Logo},
			{Tag: "TDEF", Type: blaze.TypeInteger, Value: team.DefensiveRating},
			{Tag: "TOFF", Type: blaze.TypeInteger, Value: team.OffensiveRating},
		}
		if id, valid := authoritativeCoachID(coach); valid {
			fields = append(fields, blaze.Field{
				Tag: "USEN", Type: blaze.TypeInteger, Value: dynastyCoachPortraitKeyBase + id,
			})
		}
		if coachType == 2 || coachType == 1 {
			fields = append(fields, blaze.Field{Tag: "CDSC", Type: blaze.TypeInteger, Value: team.DefensiveSchemeValue})
		}
		if coachType == 2 || coachType == 0 {
			fields = append(fields, blaze.Field{Tag: "COSC", Type: blaze.TypeInteger, Value: team.OffensiveSchemeValue})
		}
		segment := replaceAllDynastyScalarFields(result[start:end], fields)
		updated := make([]byte, 0, len(result)-(end-start)+len(segment))
		updated = append(updated, result[:start]...)
		updated = append(updated, segment...)
		updated = append(updated, result[end:]...)
		result = updated
	}
	return result
}

func (s *Service) dynastyHeadCoachForTeam(teamKey int64) (dynastyHeadCoach, bool) {
	if team, teamOK := s.authoritativeTeam(teamKey); teamOK {
		if authoritative, coachOK := s.authoritativeHeadCoach(teamKey); coachOK {
			id, idOK := authoritativeCoachID(authoritative)
			if idOK {
				return dynastyHeadCoach{
					coachKey:        dynastyCoachPortraitKeyBase + id,
					portrait:        authoritative.Portrait,
					firstName:       authoritative.FirstName,
					lastName:        authoritative.LastName,
					archetype:       authoritative.ArchetypeValue,
					defensiveScheme: team.DefensiveSchemeValue,
					offensiveScheme: team.OffensiveSchemeValue,
					level:           authoritative.Level,
					teamID:          team.PresentationID,
					pipeline:        authoritative.PipelineValue,
				}, true
			}
		}
	}
	main, _, _, ok := dynastyTeamCoachRecord(teamKey)
	if !ok {
		return dynastyHeadCoach{}, false
	}
	coachKey, ok := dynastyCoachKeyForTeam(teamKey)
	if !ok || coachKey < dynastyCoachPortraitKeyBase {
		return dynastyHeadCoach{}, false
	}
	coach := dynastyHeadCoach{coachKey: coachKey, portrait: coachKey - dynastyCoachPortraitKeyBase}
	if coach.firstName, ok = dynastyStringScalar(main, "CFNM"); !ok {
		return dynastyHeadCoach{}, false
	}
	if coach.lastName, ok = dynastyStringScalar(main, "CLNM"); !ok {
		return dynastyHeadCoach{}, false
	}
	for tag, destination := range map[string]*int64{
		"CHAT": &coach.archetype,
		"DEFS": &coach.defensiveScheme,
		"OFFS": &coach.offensiveScheme,
		"LEVL": &coach.level,
		"TMID": &coach.teamID,
	} {
		if *destination, ok = dynastyIntegerScalar(main, tag); !ok {
			return dynastyHeadCoach{}, false
		}
	}
	if authoritative, found := s.authoritativeHeadCoach(teamKey); found {
		coach.firstName = authoritative.FirstName
		coach.lastName = authoritative.LastName
		coach.level = authoritative.Level
		coach.portrait = authoritative.Portrait
		if id, valid := authoritativeCoachID(authoritative); valid {
			coach.coachKey = dynastyCoachPortraitKeyBase + id
		}
		if archetype, valid := coachArchetypeValue(authoritative.Archetype); valid {
			coach.archetype = archetype
		}
		coach.pipeline = authoritative.PipelineValue
	}
	return coach, true
}

func dynastyCoachKeyForTeam(teamKey int64) (int64, bool) {
	encodedKey, err := encodedIntegerValue(teamKey)
	if err != nil {
		return 0, false
	}
	keyOffset := bytes.LastIndex(dynastyForm133300356Payload, encodedKey)
	if keyOffset < 0 {
		return 0, false
	}
	encodedTag, _ := blaze.EncodeTag("CHKY")
	searchEnd := keyOffset
	for searchEnd > 0 {
		coachKeyOffset := bytes.LastIndex(dynastyForm133300356Payload[:searchEnd], encodedTag[:])
		if coachKeyOffset < 0 || keyOffset-coachKeyOffset > 4096 {
			break
		}
		if coachKeyOffset+5 <= len(dynastyForm133300356Payload) && blaze.Type(dynastyForm133300356Payload[coachKeyOffset+3]) == blaze.TypeInteger {
			value, _, ok := dynastyUnsignedInteger(dynastyForm133300356Payload, coachKeyOffset+4)
			if ok && value >= 547000000 && value < 548000000 {
				return int64(value), true
			}
		}
		searchEnd = coachKeyOffset
	}
	return 0, false
}

func replaceFirstDynastyScalarField(payload []byte, field blaze.Field) []byte {
	start, end, ok := dynastyScalarFieldBounds(payload, field.Tag)
	if !ok {
		return payload
	}
	replacement, err := blaze.Encode([]blaze.Field{field})
	if err != nil {
		return payload
	}
	result := make([]byte, 0, len(payload)-(end-start)+len(replacement))
	result = append(result, payload[:start]...)
	result = append(result, replacement...)
	result = append(result, payload[end:]...)
	return result
}

func dynastyIntegerScalar(record []byte, tag string) (int64, bool) {
	start, _, ok := dynastyScalarFieldBounds(record, tag)
	if !ok || blaze.Type(record[start+3]) != blaze.TypeInteger {
		return 0, false
	}
	value, _, ok := dynastyUnsignedInteger(record, start+4)
	return int64(value), ok
}

func dynastyStringScalar(record []byte, tag string) (string, bool) {
	start, end, ok := dynastyScalarFieldBounds(record, tag)
	if !ok || blaze.Type(record[start+3]) != blaze.TypeString {
		return "", false
	}
	length, valueStart, ok := dynastyUnsignedInteger(record, start+4)
	if !ok || length < 1 || valueStart+int(length) != end || record[end-1] != 0 {
		return "", false
	}
	return string(record[valueStart : end-1]), true
}

func (s *Service) selectedDynastyCoachPayload(cs *clientSession, captured []byte, requestedCoachKey int64) []byte {
	teamKey := cs.selectedTeam.Load()
	if teamKey == 0 || teamKey == capturedDynastyTeamKey {
		return append([]byte(nil), captured...)
	}
	capturedMain, _, _, ok := dynastyTeamCoachRecord(capturedDynastyTeamKey)
	if !ok {
		return s.localizeSelectedTeam(cs, captured)
	}
	selectedMain, selectedHistory, selectedHistoryCount, ok := dynastyTeamCoachRecord(teamKey)
	if !ok {
		return s.localizeSelectedTeam(cs, captured)
	}

	mainOffset := bytes.Index(captured, capturedMain)
	if mainOffset < 0 {
		return s.localizeSelectedTeam(cs, captured)
	}
	localizedMain := append([]byte(nil), selectedMain...)
	if authoritative, found := s.authoritativeCoachForSelection(teamKey, requestedCoachKey); found {
		localizedMain = replaceAuthoritativeCoachFields(localizedMain, authoritative)
	}
	result := make([]byte, 0, len(captured)-len(capturedMain)+len(localizedMain))
	result = append(result, captured[:mainOffset]...)
	result = append(result, localizedMain...)
	result = append(result, captured[mainOffset+len(capturedMain):]...)
	result = replaceDynastyCoachHistory(result, selectedHistory, selectedHistoryCount)
	return s.localizeSelectedTeam(cs, result)
}

func replaceDynastyCoachHistory(payload, selectedHistory []byte, selectedCount byte) []byte {
	tagOffset := bytes.Index(payload, dynastyCoachHistoryTag)
	if tagOffset < 0 || tagOffset+6 > len(payload) {
		return payload
	}
	originalCount := int(payload[tagOffset+5])
	if selectedCount == 0 || int(selectedCount) > originalCount {
		return payload
	}
	historyStart := tagOffset + 6
	historyEnd, ok := dynastyCompactHistoryRowsEnd(payload, historyStart, originalCount)
	if !ok {
		return payload
	}
	selectedEnd, ok := dynastyCompactHistoryRowsEnd(selectedHistory, 0, int(selectedCount))
	if !ok || (selectedEnd != len(selectedHistory) &&
		(selectedEnd+1 != len(selectedHistory) || selectedHistory[selectedEnd] != 0)) {
		return payload
	}
	selectedRows := selectedHistory[:selectedEnd]

	// The retail client allocates this compact table using the captured fixed
	// cardinality and later indexes all of its rows. Shrinking Akron's 20-row
	// table to the three rows carried by another team's form leaves null row
	// objects and crashes coach selection. Keep the fixed topology, use every
	// authoritative selected-team row, and pad unused history slots with a
	// structurally valid selected-coach row whose season/result values are empty.
	firstRowEnd, ok := dynastyCompactHistoryRowsEnd(selectedRows, 0, 1)
	if !ok {
		return payload
	}
	paddingRow := replaceAllDynastyScalarFields(selectedRows[:firstRowEnd], []blaze.Field{
		{Tag: "TMLS", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TMWL", Type: blaze.TypeString, Value: ""},
		{Tag: "TMWN", Type: blaze.TypeInteger, Value: int64(0)},
		{Tag: "TMYR", Type: blaze.TypeInteger, Value: int64(0)},
	})
	fixedHistory := make([]byte, 0, len(selectedRows)+(originalCount-int(selectedCount))*len(paddingRow))
	fixedHistory = append(fixedHistory, selectedRows...)
	for row := int(selectedCount); row < originalCount; row++ {
		fixedHistory = append(fixedHistory, paddingRow...)
	}

	result := make([]byte, 0, len(payload)-(historyEnd-historyStart)+len(fixedHistory))
	result = append(result, payload[:tagOffset+5]...)
	result = append(result, byte(originalCount))
	result = append(result, fixedHistory...)
	result = append(result, payload[historyEnd:]...)
	return result
}

func dynastyCompactHistoryRowsEnd(payload []byte, start, count int) (int, bool) {
	if count < 1 || start+4 > len(payload) {
		return 0, false
	}
	rowTag := payload[start : start+4]
	cursor := start
	lastRow := -1
	for row := 0; row < count; row++ {
		relative := bytes.Index(payload[cursor:], rowTag)
		if relative < 0 {
			return 0, false
		}
		lastRow = cursor + relative
		cursor = lastRow + len(rowTag)
	}
	cursor = lastRow
	for field := 0; field < 6; field++ {
		if cursor+5 > len(payload) {
			return 0, false
		}
		switch blaze.Type(payload[cursor+3]) {
		case blaze.TypeInteger:
			end, ok := dynastyIntegerEnd(payload, cursor+4)
			if !ok {
				return 0, false
			}
			cursor = end
		case blaze.TypeString:
			length, valueStart, ok := dynastyUnsignedInteger(payload, cursor+4)
			if !ok || length > uint64(len(payload)-valueStart) {
				return 0, false
			}
			cursor = valueStart + int(length)
		default:
			return 0, false
		}
	}
	if cursor >= len(payload) || payload[cursor] != 0 {
		return 0, false
	}
	return cursor + 1, true
}

func (s *Service) authoritativeCoachForSelection(teamKey, coachKey int64) (cfb27assets.Coach, bool) {
	if s.coachCatalog == nil || s.coachCatalogError != nil {
		return cfb27assets.Coach{}, false
	}
	if coachKey > dynastyCoachPortraitKeyBase {
		for _, coach := range s.coachCatalog.Coaches {
			id, valid := authoritativeCoachID(coach)
			if valid && dynastyCoachPortraitKeyBase+id == coachKey {
				return coach, true
			}
		}
	}
	return s.authoritativeHeadCoach(teamKey)
}

func (s *Service) authoritativeHeadCoach(teamKey int64) (cfb27assets.Coach, bool) {
	if s.coachCatalog == nil || s.coachCatalogError != nil {
		return cfb27assets.Coach{}, false
	}
	if team, ok := s.coachCatalog.TeamByKey(teamKey); ok {
		if coach, found := s.coachCatalog.HeadCoachByTeamIndex(team.TeamIndex); found {
			return coach, true
		}
	}
	identity, ok := dynastyTeamIdentityForKey(teamKey)
	if !ok {
		return cfb27assets.Coach{}, false
	}
	return s.coachCatalog.HeadCoachByName(identity.coachFirstName, identity.coachLastName)
}

func replaceAuthoritativeCoachFields(record []byte, coach cfb27assets.Coach) []byte {
	fields := []blaze.Field{
		{Tag: "CFNM", Type: blaze.TypeString, Value: coach.FirstName},
		{Tag: "CLNM", Type: blaze.TypeString, Value: coach.LastName},
		{Tag: "CHPR", Type: blaze.TypeInteger, Value: coach.PrestigeValue},
		{Tag: "CSXP", Type: blaze.TypeInteger, Value: coach.Portrait},
		{Tag: "LEVL", Type: blaze.TypeInteger, Value: coach.Level},
		{Tag: "CPOS", Type: blaze.TypeInteger, Value: coach.PositionValue},
		{Tag: "COAC", Type: blaze.TypeInteger, Value: coach.ArchetypeValue},
	}
	if coachType, ok := coachWireRoleValue(coach.Position); ok {
		fields = append(fields, blaze.Field{Tag: "CHAT", Type: blaze.TypeInteger, Value: coachType})
	}
	if id, ok := authoritativeCoachID(coach); ok {
		fields = append(fields, blaze.Field{Tag: "CCID", Type: blaze.TypeInteger, Value: id})
	}
	result := append([]byte(nil), record...)
	for _, field := range fields {
		start, end, ok := dynastyScalarFieldBounds(result, field.Tag)
		if !ok {
			continue
		}
		replacement, err := blaze.Encode([]blaze.Field{field})
		if err != nil {
			continue
		}
		updated := make([]byte, 0, len(result)-(end-start)+len(replacement))
		updated = append(updated, result[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, result[end:]...)
		result = updated
	}
	return result
}

func authoritativeCoachID(coach cfb27assets.Coach) (int64, bool) {
	separator := strings.LastIndexByte(coach.AssetName, '_')
	if separator < 0 || separator+1 >= len(coach.AssetName) {
		return 0, false
	}
	value, err := strconv.ParseInt(coach.AssetName[separator+1:], 10, 64)
	return value, err == nil && value > 0
}

func coachArchetypeValue(name string) (int64, bool) {
	values := map[string]int64{
		"Motivator": 0, "Schemer": 1, "Recruiter": 2, "MasterMotivator": 3,
		"Architect": 4, "SchemeGuru": 5, "Strategist": 6, "EliteRecruiter": 7,
		"TalentDeveloper": 8, "Rainmaker": 9, "Visionary": 10, "ProgramBuilder": 11, "CEO": 12,
	}
	value, ok := values[name]
	return value, ok
}

func coachPositionValue(name string) (int64, bool) {
	values := map[string]int64{"HeadCoach": 0, "OffensiveCoordinator": 1, "DefensiveCoordinator": 2}
	value, ok := values[name]
	return value, ok
}

func coachWireRoleValue(name string) (int64, bool) {
	values := map[string]int64{"HeadCoach": 2, "OffensiveCoordinator": 0, "DefensiveCoordinator": 1}
	value, ok := values[name]
	return value, ok
}

func dynastyScalarFieldBounds(record []byte, tag string) (int, int, bool) {
	encodedTag, err := blaze.EncodeTag(tag)
	if err != nil {
		return 0, 0, false
	}
	start := bytes.Index(record, encodedTag[:])
	if start < 0 || start+5 > len(record) {
		return 0, 0, false
	}
	valueStart := start + 4
	switch blaze.Type(record[start+3]) {
	case blaze.TypeInteger:
		end, ok := dynastyIntegerEnd(record, valueStart)
		return start, end, ok
	case blaze.TypeString:
		length, lengthEnd, ok := dynastyUnsignedInteger(record, valueStart)
		if !ok || length > uint64(len(record)-lengthEnd) {
			return 0, 0, false
		}
		return start, lengthEnd + int(length), true
	default:
		return 0, 0, false
	}
}

func dynastyIntegerEnd(data []byte, start int) (int, bool) {
	_, end, ok := dynastyUnsignedInteger(data, start)
	return end, ok
}

func dynastyUnsignedInteger(data []byte, start int) (uint64, int, bool) {
	if start >= len(data) {
		return 0, 0, false
	}
	first := data[start]
	value := uint64(first & 0x3f)
	shift := uint(6)
	end := start + 1
	for first&0x80 != 0 {
		if end >= len(data) || shift >= 64 {
			return 0, 0, false
		}
		first = data[end]
		end++
		value |= uint64(first&0x7f) << shift
		shift += 7
	}
	return value, end, true
}

func dynastyTeamCoachRecord(teamKey int64) (main []byte, history []byte, historyCount byte, ok bool) {
	encodedKey, err := encodedIntegerValue(teamKey)
	if err != nil {
		return nil, nil, 0, false
	}
	keyOffset := bytes.LastIndex(dynastyForm133300356Payload, encodedKey)
	if keyOffset < 0 {
		return nil, nil, 0, false
	}
	mainTagOffset := bytes.LastIndex(dynastyForm133300356Payload[:keyOffset], dynastyTeamMainTag)
	if mainTagOffset < 1 {
		return nil, nil, 0, false
	}
	mainOffset := mainTagOffset - 1
	historyTagRelative := bytes.Index(dynastyForm133300356Payload[mainOffset:keyOffset], dynastyTeamHistoryTag)
	if historyTagRelative < 0 {
		return nil, nil, 0, false
	}
	historyTagOffset := mainOffset + historyTagRelative
	if historyTagOffset+6 > len(dynastyForm133300356Payload) {
		return nil, nil, 0, false
	}
	historyOffset := historyTagOffset + 6
	historyEndRelative := bytes.Index(dynastyForm133300356Payload[historyOffset:keyOffset], dynastyTeamAfterHistory)
	if historyEndRelative < 0 {
		return nil, nil, 0, false
	}
	historyEnd := historyOffset + historyEndRelative
	return append([]byte(nil), dynastyForm133300356Payload[mainOffset:historyTagOffset]...),
		append([]byte(nil), dynastyForm133300356Payload[historyOffset:historyEnd]...),
		dynastyForm133300356Payload[historyTagOffset+5], true
}

func encodedIntegerValue(value int64) ([]byte, error) {
	payload, err := blaze.Encode([]blaze.Field{{Tag: "VALU", Type: blaze.TypeInteger, Value: value}})
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), payload[4:]...), nil
}
