package cfb27blaze

import (
	"bytes"
	"fmt"
	"sort"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27assets"
)

const dynastyPlayerRecordKeyBase int64 = 556793856

type dynastyRosterRecord struct {
	keyStart  int
	bodyStart int
}

func (s *Service) selectedDynastyRosterPayload(cs *clientSession, captured []byte) []byte {
	teamKey := cs.selectedTeam.Load()
	if teamKey == 0 || s.coachCatalog == nil || s.coachCatalogError != nil {
		return s.localizeSelectedTeam(cs, captured)
	}
	team, ok := s.coachCatalog.TeamByKey(teamKey)
	if !ok {
		return s.localizeSelectedTeam(cs, captured)
	}
	players := s.coachCatalog.PlayersByTeamIndex(team.TeamIndex)
	if len(players) == 0 {
		return s.localizeSelectedTeam(cs, captured)
	}
	return s.localizeSelectedTeam(cs, replaceDynastyRosterPlayers(captured, team, players))
}

func replaceDynastyRosterPlayers(payload []byte, team cfb27assets.Team, players []cfb27assets.Player) []byte {
	records := dynastyRosterRecords(payload)
	if len(records) == 0 || len(players) == 0 {
		return append([]byte(nil), payload...)
	}
	ordered := append([]cfb27assets.Player(nil), players...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OverallRating != ordered[j].OverallRating {
			return ordered[i].OverallRating > ordered[j].OverallRating
		}
		return ordered[i].ID < ordered[j].ID
	})
	assignments := assignDynastyRosterPlayers(payload, records, ordered)

	result := append([]byte(nil), payload...)
	for index := len(records) - 1; index >= 0; index-- {
		end := len(result)
		if index+1 < len(records) {
			end = records[index+1].keyStart
		}
		bodyOffset := records[index].bodyStart - records[index].keyStart
		body := overlayDynastyPlayerRecord(result[records[index].bodyStart:end], team, assignments[index])
		key, err := encodedIntegerValue(int64(assignments[index].ID))
		if err != nil {
			continue
		}
		prefix := result[records[index].keyStart:records[index].bodyStart]
		_, oldKeyEnd, ok := dynastyUnsignedInteger(prefix, 0)
		if !ok || oldKeyEnd > bodyOffset {
			continue
		}
		segment := make([]byte, 0, len(key)+len(prefix)-oldKeyEnd+len(body))
		segment = append(segment, key...)
		segment = append(segment, prefix[oldKeyEnd:]...)
		segment = append(segment, body...)
		updated := make([]byte, 0, len(result)-(end-records[index].keyStart)+len(segment))
		updated = append(updated, result[:records[index].keyStart]...)
		updated = append(updated, segment...)
		updated = append(updated, result[end:]...)
		result = updated
	}
	return result
}

func dynastyRosterRecords(payload []byte) []dynastyRosterRecord {
	tag, err := blaze.EncodeTag("BACC")
	if err != nil {
		return nil
	}
	records := make([]dynastyRosterRecord, 0, 100)
	for cursor := 0; cursor+4 <= len(payload); {
		relative := bytes.Index(payload[cursor:], tag[:])
		if relative < 0 {
			break
		}
		start := cursor + relative
		if start+4 < len(payload) {
			// Each compact Dynasty database map entry is encoded as:
			// <player row key><record marker 0x01><five-byte Player type id><BACC...>.
			marker := start - 6
			keyStart := marker - 1
			for keyStart > 0 && payload[keyStart-1]&0x80 != 0 {
				keyStart--
			}
			if marker >= 1 && payload[marker] == 1 {
				records = append(records, dynastyRosterRecord{keyStart: keyStart, bodyStart: start})
			}
		}
		cursor = start + len(tag)
	}
	return records
}

func assignDynastyRosterPlayers(payload []byte, records []dynastyRosterRecord, players []cfb27assets.Player) []cfb27assets.Player {
	assigned := make([]cfb27assets.Player, len(records))
	used := make([]bool, len(players))
	for index, record := range records {
		end := len(payload)
		if index+1 < len(records) {
			end = records[index+1].keyStart
		}
		position, hasPosition := dynastyIntegerScalar(payload[record.bodyStart:end], "POSI")
		selected := -1
		if hasPosition {
			for playerIndex := range players {
				if !used[playerIndex] && players[playerIndex].PositionValue == position {
					selected = playerIndex
					break
				}
			}
		}
		if selected < 0 {
			for playerIndex := range players {
				if !used[playerIndex] {
					selected = playerIndex
					break
				}
			}
		}
		if selected < 0 {
			selected = index % len(players)
		}
		assigned[index] = players[selected]
		used[selected] = true
	}
	return assigned
}

func overlayDynastyPlayerRecord(record []byte, team cfb27assets.Team, player cfb27assets.Player) []byte {
	fields := []blaze.Field{
		{Tag: "FNAM", Type: blaze.TypeString, Value: player.FirstName},
		{Tag: "LNAM", Type: blaze.TypeString, Value: player.LastName},
		{Tag: "JNUM", Type: blaze.TypeInteger, Value: player.JerseyNum},
		{Tag: "POSI", Type: blaze.TypeInteger, Value: player.PositionValue},
		{Tag: "POIN", Type: blaze.TypeInteger, Value: player.OverallRating},
		{Tag: "POVM", Type: blaze.TypeInteger, Value: player.OverallRating},
		{Tag: "POVR", Type: blaze.TypeInteger, Value: player.OverallRating},
		{Tag: "PORT", Type: blaze.TypeInteger, Value: player.Portrait},
		{Tag: "PGID", Type: blaze.TypeInteger, Value: player.PresentationID},
		{Tag: "PHGT", Type: blaze.TypeInteger, Value: player.Height},
		{Tag: "PWGT", Type: blaze.TypeInteger, Value: player.Weight},
		{Tag: "SHYR", Type: blaze.TypeInteger, Value: player.ClassYear},
		{Tag: "TDEV", Type: blaze.TypeInteger, Value: player.DevelopmentTrait},
		{Tag: "RDKY", Type: blaze.TypeInteger, Value: dynastyPlayerRecordKeyBase + int64(player.ID)},
		{Tag: "TEAM", Type: blaze.TypeInteger, Value: team.TeamKey},
		{Tag: "TMLG", Type: blaze.TypeInteger, Value: team.Logo},
	}
	if player.Hometown != "" {
		fields = append(fields, blaze.Field{Tag: "HOME", Type: blaze.TypeString, Value: player.Hometown})
	}
	for tag, value := range player.Ratings {
		if len(tag) != 4 || tag[0] != 'P' {
			continue
		}
		fields = append(fields,
			blaze.Field{Tag: tag, Type: blaze.TypeInteger, Value: value},
			blaze.Field{Tag: "B" + tag[1:], Type: blaze.TypeInteger, Value: value},
		)
	}
	result := replaceAllDynastyScalarFields(record, fields)
	return replaceDynastyPlayerShortName(result, fmt.Sprintf("%s.%s", player.FirstName[:1], player.LastName))
}

func replaceDynastyPlayerShortName(record []byte, value string) []byte {
	lastNameTag, _ := blaze.EncodeTag("LNAM")
	nameTag, _ := blaze.EncodeTag("NAME")
	lastName := bytes.Index(record, lastNameTag[:])
	if lastName < 0 {
		return record
	}
	searchEnd := lastName + 128
	if searchEnd > len(record) {
		searchEnd = len(record)
	}
	relative := bytes.Index(record[lastName:searchEnd], nameTag[:])
	if relative < 0 {
		return record
	}
	start := lastName + relative
	_, end, ok := dynastyScalarFieldBounds(record[start:], "NAME")
	if !ok {
		return record
	}
	replacement, err := blaze.Encode([]blaze.Field{{Tag: "NAME", Type: blaze.TypeString, Value: value}})
	if err != nil {
		return record
	}
	end += start
	result := make([]byte, 0, len(record)-(end-start)+len(replacement))
	result = append(result, record[:start]...)
	result = append(result, replacement...)
	result = append(result, record[end:]...)
	return result
}
