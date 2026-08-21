package cfb27assets

import (
	"os"
	"path/filepath"
	"testing"
)

const validCatalog = `{
  "version": 1,
  "source": {"assetRoot":"C:/assets","slot":0,"dataRevisionVersion":3,"dynastySha256":"abc","coachSchemaSha256":"def","teamSchemaSha256":"ghi"},
  "coaches": [
    {"id":665,"firstName":"Ryan","lastName":"Day","assetName":"Unique_C_DayRyan_665","portrait":618,"level":76,"position":"HeadCoach","positionValue":0,"teamIndex":68,"pipeline":"Ohio","pipelineValue":28,"archetype":"CEO","archetypeValue":12,"data":{"FirstName":"Ryan"}},
    {"id":668,"firstName":"Brent","lastName":"Venables","assetName":"Unique_C_VenablesBrent_668","portrait":898,"level":59,"position":"HeadCoach","positionValue":0,"teamIndex":69,"pipeline":"Kansas","pipelineValue":12,"archetype":"Architect","archetypeValue":4,"data":{"FirstName":"Brent"}},
    {"id":900,"firstName":"Brian","lastName":"Hartline","assetName":"Unique_C_HartlineBrian_900","portrait":411,"level":34,"position":"OffensiveCoordinator","positionValue":1,"teamIndex":68,"pipeline":"Ohio","pipelineValue":28,"archetype":"Recruiter","archetypeValue":2,"data":{"FirstName":"Brian"}}
  ],
  "teams": [
    {"id":1,"teamKey":830865409,"teamIndex":1,"presentationId":1101,"assetName":"AKRONX","displayName":"Akron","longName":"Akron","nickname":"Zips","shortName":"AKRN","logo":1,"defensiveRating":66,"offensiveRating":66,"overallRating":66,"offensiveScheme":"OFF_SPREAD","offensiveSchemeValue":6,"defensiveScheme":"DEF_3_4_MULTIPLE","defensiveSchemeValue":17,"prestigeRank":137,"teamPrestige":0,"primaryColor":269891,"secondaryColor":13023108,"conference":{"name":"MAC","enum":"MAC","presentationId":6},"data":{"TeamIndex":1}},
    {"id":87,"teamKey":830865495,"teamIndex":68,"presentationId":1178,"assetName":"OHIOST","displayName":"Ohio State","longName":"Ohio State","nickname":"Buckeyes","nicknameAlt":"Bucks","shortName":"OSU","logo":78,"defensiveRating":96,"offensiveRating":94,"overallRating":94,"offensiveScheme":"OFF_SPREAD","offensiveSchemeValue":6,"defensiveScheme":"DEF_4_2_5","defensiveSchemeValue":14,"prestigeRank":3,"teamPrestige":10,"primaryColor":12649008,"secondaryColor":10660269,"conference":{"name":"Big Ten","enum":"BigTen","presentationId":1},"data":{"TeamIndex":68}}
  ]
}`

func TestLoadCoachCatalogResolvesAuthoritativeHeadCoachAndAllStaff(t *testing.T) {
	catalog, err := loadFixture(t, validCatalog)
	if err != nil {
		t.Fatal(err)
	}

	coach, ok := catalog.HeadCoachByName(" ryan ", "DAY")
	if !ok {
		t.Fatal("Ryan Day was not indexed")
	}
	if coach.Portrait != 618 || coach.AssetName != "Unique_C_DayRyan_665" || coach.Pipeline != "Ohio" || coach.Archetype != "CEO" {
		t.Fatalf("unexpected Ryan Day record: %#v", coach)
	}
	if coach.PipelineValue != 28 || coach.ArchetypeValue != 12 || coach.PositionValue != 0 {
		t.Fatalf("unexpected Ryan Day enum values: %#v", coach)
	}

	staff := catalog.CoachesByTeamIndex(68)
	if len(staff) != 2 || staff[0].Position != "HeadCoach" || staff[1].Position != "OffensiveCoordinator" {
		t.Fatalf("team 68 staff = %#v", staff)
	}
}

func TestLoadCoachCatalogResolvesAuthoritativeTeamAndHeadCoachByTeam(t *testing.T) {
	catalog, err := loadFixture(t, validCatalog)
	if err != nil {
		t.Fatal(err)
	}

	team, ok := catalog.TeamByKey(830865495)
	if !ok {
		t.Fatal("Ohio State was not indexed by Dynasty team key")
	}
	if team.PresentationID != 1178 || team.Logo != 78 || team.DefensiveRating != 96 || team.OffensiveRating != 94 || team.PrimaryColor != 0xc10230 {
		t.Fatalf("unexpected Ohio State record: %#v", team)
	}
	if team.Conference.Name != "Big Ten" || team.Conference.PresentationID != 1 {
		t.Fatalf("unexpected Ohio State conference: %#v", team.Conference)
	}
	byIndex, ok := catalog.TeamByIndex(68)
	if !ok || byIndex.TeamKey != team.TeamKey {
		t.Fatalf("team-index lookup = %#v, %v", byIndex, ok)
	}
	coach, ok := catalog.HeadCoachByTeamIndex(68)
	if !ok || coach.FirstName != "Ryan" || coach.LastName != "Day" {
		t.Fatalf("Ohio State head coach = %#v, %v", coach, ok)
	}
}

func TestLoadCoachCatalogRejectsUnsupportedVersion(t *testing.T) {
	_, err := loadFixture(t, `{"version":2,"source":{"slot":0},"coaches":[]}`)
	if err == nil {
		t.Fatal("unsupported cache version was accepted")
	}
}

func TestLoadCoachCatalogRejectsAmbiguousHeadCoachName(t *testing.T) {
	_, err := loadFixture(t, `{
      "version":1,
      "source":{"slot":0,"dynastySha256":"a","coachSchemaSha256":"b"},
      "coaches":[
        {"id":1,"firstName":"Ryan","lastName":"Day","position":"HeadCoach","teamIndex":68},
        {"id":2,"firstName":"Ryan","lastName":"Day","position":"HeadCoach","teamIndex":69}
      ]
    }`)
	if err == nil {
		t.Fatal("ambiguous head-coach name was accepted")
	}
}

func TestLoadCoachCatalogRetainsButDoesNotIndexUnassignedPlaceholder(t *testing.T) {
	catalog, err := loadFixture(t, `{
      "version":1,
      "source":{"slot":0,"dynastySha256":"a","coachSchemaSha256":"b"},
      "coaches":[
        {"id":109,"firstName":"","lastName":"","position":"DefensiveCoordinator","teamIndex":255}
      ]
    }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Coaches) != 1 || len(catalog.CoachesByTeamIndex(255)) != 0 {
		t.Fatalf("unassigned placeholder was not retained without indexing: %#v", catalog.Coaches)
	}
}

func loadFixture(t *testing.T, contents string) (*CoachCatalog, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coaches.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadCoachCatalog(path)
}
