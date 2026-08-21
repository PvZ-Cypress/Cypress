package cfb27assets

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const CoachCatalogVersion = 1

type CoachCatalog struct {
	Version int                `json:"version"`
	Source  CoachCatalogSource `json:"source"`
	Coaches []Coach            `json:"coaches"`
	Teams   []Team             `json:"teams"`
	Players []Player           `json:"players"`

	headCoachByName map[string]int
	headCoachByTeam map[int]int
	coachesByTeam   map[int][]int
	teamByKey       map[int64]int
	teamByIndex     map[int]int
	playersByTeam   map[int][]int
}

type CoachCatalogSource struct {
	AssetRoot           string `json:"assetRoot"`
	Slot                int    `json:"slot"`
	DataRevisionVersion int    `json:"dataRevisionVersion"`
	DynastySHA256       string `json:"dynastySha256"`
	CoachSchemaSHA256   string `json:"coachSchemaSha256"`
	TeamSchemaSHA256    string `json:"teamSchemaSha256"`
	PlayerSchemaSHA256  string `json:"playerSchemaSha256"`
}

type Coach struct {
	ID               int            `json:"id"`
	FirstName        string         `json:"firstName"`
	LastName         string         `json:"lastName"`
	AssetName        string         `json:"assetName"`
	Portrait         int64          `json:"portrait"`
	Level            int64          `json:"level"`
	Position         string         `json:"position"`
	PositionValue    int64          `json:"positionValue"`
	TeamIndex        int            `json:"teamIndex"`
	Pipeline         string         `json:"pipeline"`
	PipelineValue    int64          `json:"pipelineValue"`
	Archetype        string         `json:"archetype"`
	ArchetypeValue   int64          `json:"archetypeValue"`
	CoachPrestige    string         `json:"coachPrestige"`
	PrestigeValue    int64          `json:"coachPrestigeValue"`
	CharacterVisuals CoachVisuals   `json:"characterVisuals"`
	OffensiveScheme  any            `json:"offensiveScheme"`
	DefensiveScheme  any            `json:"defensiveScheme"`
	Data             map[string]any `json:"data"`
}

type CoachVisuals struct {
	TableID   int    `json:"tableId"`
	RowNumber int    `json:"rowNumber"`
	RawData   string `json:"rawData"`
}

type Team struct {
	ID                   int            `json:"id"`
	TeamKey              int64          `json:"teamKey"`
	TeamIndex            int            `json:"teamIndex"`
	PresentationID       int64          `json:"presentationId"`
	AssetName            string         `json:"assetName"`
	DisplayName          string         `json:"displayName"`
	LongName             string         `json:"longName"`
	Nickname             string         `json:"nickname"`
	NicknameAlt          string         `json:"nicknameAlt"`
	ShortName            string         `json:"shortName"`
	Logo                 int64          `json:"logo"`
	DefensiveRating      int64          `json:"defensiveRating"`
	OffensiveRating      int64          `json:"offensiveRating"`
	OverallRating        int64          `json:"overallRating"`
	OffensiveScheme      string         `json:"offensiveScheme"`
	OffensiveSchemeValue int64          `json:"offensiveSchemeValue"`
	DefensiveScheme      string         `json:"defensiveScheme"`
	DefensiveSchemeValue int64          `json:"defensiveSchemeValue"`
	PrestigeRank         int64          `json:"prestigeRank"`
	TeamPrestige         int64          `json:"teamPrestige"`
	PrimaryColor         int64          `json:"primaryColor"`
	SecondaryColor       int64          `json:"secondaryColor"`
	Conference           TeamConference `json:"conference"`
	Data                 map[string]any `json:"data"`
}

type TeamConference struct {
	Name           string `json:"name"`
	Enum           string `json:"enum"`
	PresentationID int64  `json:"presentationId"`
}

type Player struct {
	ID               int              `json:"id"`
	FirstName        string           `json:"firstName"`
	LastName         string           `json:"lastName"`
	TeamIndex        int              `json:"teamIndex"`
	Position         string           `json:"position"`
	PositionValue    int64            `json:"positionValue"`
	JerseyNum        int64            `json:"jerseyNum"`
	OverallRating    int64            `json:"overallRating"`
	Portrait         int64            `json:"portrait"`
	PresentationID   int64            `json:"presentationId"`
	GenericHead      int64            `json:"genericHead"`
	Height           int64            `json:"height"`
	Weight           int64            `json:"weight"`
	ClassYear        int64            `json:"classYear"`
	DevelopmentTrait int64            `json:"developmentTrait"`
	Hometown         string           `json:"hometown"`
	Ratings          map[string]int64 `json:"ratings"`
}

func LoadCoachCatalog(path string) (*CoachCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CFB27 coach catalog: %w", err)
	}
	var catalog CoachCatalog
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode CFB27 coach catalog: %w", err)
	}
	if catalog.Version != CoachCatalogVersion {
		return nil, fmt.Errorf("unsupported CFB27 coach catalog version %d", catalog.Version)
	}
	if catalog.Source.DynastySHA256 == "" || catalog.Source.CoachSchemaSHA256 == "" {
		return nil, fmt.Errorf("CFB27 coach catalog is missing source hashes")
	}
	if len(catalog.Coaches) == 0 {
		return nil, fmt.Errorf("CFB27 coach catalog contains no coaches")
	}
	if len(catalog.Teams) > 0 && catalog.Source.TeamSchemaSHA256 == "" {
		return nil, fmt.Errorf("CFB27 coach catalog contains teams but is missing the team schema hash")
	}

	catalog.headCoachByName = make(map[string]int)
	catalog.headCoachByTeam = make(map[int]int)
	catalog.coachesByTeam = make(map[int][]int)
	catalog.teamByKey = make(map[int64]int)
	catalog.teamByIndex = make(map[int]int)
	catalog.playersByTeam = make(map[int][]int)
	for index := range catalog.Coaches {
		coach := &catalog.Coaches[index]
		coach.FirstName = strings.TrimSpace(coach.FirstName)
		coach.LastName = strings.TrimSpace(coach.LastName)
		coach.Position = strings.TrimSpace(coach.Position)
		if coach.FirstName == "" || coach.LastName == "" || coach.Position == "" {
			if coach.TeamIndex == 255 {
				continue
			}
			return nil, fmt.Errorf("coach record %d is missing identity or position", coach.ID)
		}
		catalog.coachesByTeam[coach.TeamIndex] = append(catalog.coachesByTeam[coach.TeamIndex], index)
		if strings.EqualFold(coach.Position, "HeadCoach") {
			key := coachNameKey(coach.FirstName, coach.LastName)
			if previous, exists := catalog.headCoachByName[key]; exists {
				return nil, fmt.Errorf("ambiguous head coach %s %s in records %d and %d",
					coach.FirstName, coach.LastName, catalog.Coaches[previous].ID, coach.ID)
			}
			catalog.headCoachByName[key] = index
			if coach.TeamIndex != 255 {
				if previous, exists := catalog.headCoachByTeam[coach.TeamIndex]; exists {
					return nil, fmt.Errorf("ambiguous head coaches for team index %d in records %d and %d",
						coach.TeamIndex, catalog.Coaches[previous].ID, coach.ID)
				}
				catalog.headCoachByTeam[coach.TeamIndex] = index
			}
		}
	}
	for index := range catalog.Teams {
		team := &catalog.Teams[index]
		team.AssetName = strings.TrimSpace(team.AssetName)
		team.DisplayName = strings.TrimSpace(team.DisplayName)
		team.LongName = strings.TrimSpace(team.LongName)
		team.Nickname = strings.TrimSpace(team.Nickname)
		team.NicknameAlt = strings.TrimSpace(team.NicknameAlt)
		team.ShortName = strings.TrimSpace(team.ShortName)
		team.Conference.Name = strings.TrimSpace(team.Conference.Name)
		team.Conference.Enum = strings.TrimSpace(team.Conference.Enum)
		if team.TeamKey <= 0 {
			return nil, fmt.Errorf("team record %d has invalid Dynasty key %d", team.ID, team.TeamKey)
		}
		if previous, exists := catalog.teamByKey[team.TeamKey]; exists {
			return nil, fmt.Errorf("duplicate Dynasty team key %d in records %d and %d",
				team.TeamKey, catalog.Teams[previous].ID, team.ID)
		}
		catalog.teamByKey[team.TeamKey] = index
		if team.TeamIndex != 255 {
			if previous, exists := catalog.teamByIndex[team.TeamIndex]; exists {
				return nil, fmt.Errorf("duplicate team index %d in records %d and %d",
					team.TeamIndex, catalog.Teams[previous].ID, team.ID)
			}
			catalog.teamByIndex[team.TeamIndex] = index
		}
	}
	for team := range catalog.coachesByTeam {
		sort.SliceStable(catalog.coachesByTeam[team], func(i, j int) bool {
			left := catalog.Coaches[catalog.coachesByTeam[team][i]]
			right := catalog.Coaches[catalog.coachesByTeam[team][j]]
			return coachPositionOrder(left.Position) < coachPositionOrder(right.Position)
		})
	}
	for index := range catalog.Players {
		player := &catalog.Players[index]
		player.FirstName = strings.TrimSpace(player.FirstName)
		player.LastName = strings.TrimSpace(player.LastName)
		player.Position = strings.TrimSpace(player.Position)
		if player.FirstName == "" || player.LastName == "" || player.Position == "" {
			continue
		}
		catalog.playersByTeam[player.TeamIndex] = append(catalog.playersByTeam[player.TeamIndex], index)
	}
	return &catalog, nil
}

func (c *CoachCatalog) HeadCoachByTeamIndex(teamIndex int) (Coach, bool) {
	if c == nil {
		return Coach{}, false
	}
	index, ok := c.headCoachByTeam[teamIndex]
	if !ok {
		return Coach{}, false
	}
	return c.Coaches[index], true
}

func (c *CoachCatalog) TeamByKey(teamKey int64) (Team, bool) {
	if c == nil {
		return Team{}, false
	}
	index, ok := c.teamByKey[teamKey]
	if !ok {
		return Team{}, false
	}
	return c.Teams[index], true
}

func (c *CoachCatalog) TeamByIndex(teamIndex int) (Team, bool) {
	if c == nil {
		return Team{}, false
	}
	index, ok := c.teamByIndex[teamIndex]
	if !ok {
		return Team{}, false
	}
	return c.Teams[index], true
}

func (c *CoachCatalog) HeadCoachByName(firstName, lastName string) (Coach, bool) {
	if c == nil {
		return Coach{}, false
	}
	index, ok := c.headCoachByName[coachNameKey(firstName, lastName)]
	if !ok {
		return Coach{}, false
	}
	return c.Coaches[index], true
}

func (c *CoachCatalog) CoachesByTeamIndex(teamIndex int) []Coach {
	if c == nil {
		return nil
	}
	indices := c.coachesByTeam[teamIndex]
	result := make([]Coach, 0, len(indices))
	for _, index := range indices {
		result = append(result, c.Coaches[index])
	}
	return result
}

func (c *CoachCatalog) PlayersByTeamIndex(teamIndex int) []Player {
	if c == nil {
		return nil
	}
	indices := c.playersByTeam[teamIndex]
	result := make([]Player, 0, len(indices))
	for _, index := range indices {
		result = append(result, c.Players[index])
	}
	return result
}

func coachNameKey(firstName, lastName string) string {
	return strings.ToLower(strings.TrimSpace(firstName)) + "\x00" + strings.ToLower(strings.TrimSpace(lastName))
}

func coachPositionOrder(position string) int {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "headcoach":
		return 0
	case "offensivecoordinator":
		return 1
	case "defensivecoordinator":
		return 2
	default:
		return 3
	}
}
