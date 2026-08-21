import assert from 'node:assert/strict';
import test from 'node:test';

import { exportDynastyAssets } from './main.mjs';

const assetRoot = 'C:/Users/Shadow/Desktop/CFB27/Release/Dynasty_Assets';

test('exports authoritative coaches with unique portraits and complete active coverage', async () => {
  const catalog = await exportDynastyAssets({ assetRoot, slot: 0 });

  assert.equal(catalog.version, 1);
  assert.equal(catalog.coaches.length, 433);

  const ryanDay = catalog.coaches.find(
    (coach) => coach.firstName === 'Ryan' && coach.lastName === 'Day',
  );
  assert.deepEqual(
    {
      assetName: ryanDay?.assetName,
      portrait: ryanDay?.portrait,
      level: ryanDay?.level,
      position: ryanDay?.position,
      teamIndex: ryanDay?.teamIndex,
      pipeline: ryanDay?.pipeline,
      pipelineValue: ryanDay?.pipelineValue,
      archetype: ryanDay?.archetype,
      archetypeValue: ryanDay?.archetypeValue,
      positionValue: ryanDay?.positionValue,
      coachPrestige: ryanDay?.coachPrestige,
      coachPrestigeValue: ryanDay?.coachPrestigeValue,
      // Table ids shift whenever a game update inserts tables, so assert on the
      // visuals payload rather than pinning the id to one build.
      characterVisualsRow: ryanDay?.characterVisuals?.rowNumber,
      characterVisualsData: ryanDay?.characterVisuals?.rawData,
    },
    {
      assetName: 'Unique_C_DayRyan_665',
      portrait: 618,
      level: 76,
      position: 'HeadCoach',
      teamIndex: 68,
      pipeline: 'Ohio',
      pipelineValue: 28,
      archetype: 'CEO',
      archetypeValue: 12,
      positionValue: 0,
      coachPrestige: 'Aplus',
      coachPrestigeValue: 0,
      characterVisualsRow: 2872,
      characterVisualsData:
        '{"bodyType":2,"loadouts":[{"loadoutType":"CoachOnField","loadoutCategory":"CoachApparel","loadoutElements":[{"slotType":"LeftShoe","itemAssetName":"CoachWardrobe_NikeDunkLow"},{"slotType":"RightShoe","itemAssetName":"CoachWardrobe_NikeDunkLow"},{"slotType":"OuterShirt","itemAssetName":"CoachWardrobe_Polo_Black"},{"slotType":"OuterPants","itemAssetName":"CoachWardrobe_Grey_Pants"},{"slotType":"HeadWear","itemAssetName":"UC_Hat_None"},{"slotType":"EarWear","itemAssetName":"CoachWardrobe_Headset_Generic_R"}]}]}',
    },
  );
  assert.ok(
    Number(ryanDay?.characterVisuals?.tableId) > 0,
    'CharacterVisuals must resolve to a real table',
  );

  const brentVenables = catalog.coaches.find(
    (coach) => coach.firstName === 'Brent' && coach.lastName === 'Venables',
  );
  assert.deepEqual(
    {
      assetName: brentVenables?.assetName,
      portrait: brentVenables?.portrait,
      level: brentVenables?.level,
      position: brentVenables?.position,
      teamIndex: brentVenables?.teamIndex,
      pipeline: brentVenables?.pipeline,
      pipelineValue: brentVenables?.pipelineValue,
      archetype: brentVenables?.archetype,
      archetypeValue: brentVenables?.archetypeValue,
    },
    {
      assetName: 'Unique_C_VenablesBrent_668',
      portrait: 898,
      level: 59,
      position: 'HeadCoach',
      teamIndex: 69,
      pipeline: 'Kansas',
      pipelineValue: 12,
      archetype: 'Architect',
      archetypeValue: 4,
    },
  );

  assert.ok(Object.keys(ryanDay?.data ?? {}).length >= 137);
  assert.ok(Object.keys(brentVenables?.data ?? {}).length >= 137);
});

test('exports all teams with authoritative presentation, branding, ratings, schemes, and conference data', async () => {
  const catalog = await exportDynastyAssets({ assetRoot, slot: 0 });

  assert.equal(catalog.teams.length, 143);
  assert.match(catalog.source.teamSchemaSha256, /^[a-f0-9]{64}$/);

  const ohioState = catalog.teams.find((team) => team.teamKey === 830865495);
  assert.deepEqual(
    {
      id: ohioState?.id,
      teamIndex: ohioState?.teamIndex,
      presentationId: ohioState?.presentationId,
      assetName: ohioState?.assetName,
      displayName: ohioState?.displayName,
      longName: ohioState?.longName,
      nickname: ohioState?.nickname,
      nicknameAlt: ohioState?.nicknameAlt,
      shortName: ohioState?.shortName,
      logo: ohioState?.logo,
      defensiveRating: ohioState?.defensiveRating,
      offensiveRating: ohioState?.offensiveRating,
      overallRating: ohioState?.overallRating,
      offensiveScheme: ohioState?.offensiveScheme,
      offensiveSchemeValue: ohioState?.offensiveSchemeValue,
      defensiveScheme: ohioState?.defensiveScheme,
      defensiveSchemeValue: ohioState?.defensiveSchemeValue,
      prestigeRank: ohioState?.prestigeRank,
      teamPrestige: ohioState?.teamPrestige,
      primaryColor: ohioState?.primaryColor,
      secondaryColor: ohioState?.secondaryColor,
      conference: ohioState?.conference,
    },
    {
      id: 87,
      teamIndex: 68,
      presentationId: 1178,
      assetName: 'OHIOST',
      displayName: 'Ohio State',
      longName: 'Ohio State',
      nickname: 'Buckeyes',
      nicknameAlt: 'Bucks',
      shortName: 'OSU',
      logo: 78,
      defensiveRating: 96,
      offensiveRating: 94,
      overallRating: 94,
      offensiveScheme: 'OFF_SPREAD',
      offensiveSchemeValue: 6,
      defensiveScheme: 'DEF_4_2_5',
      defensiveSchemeValue: 14,
      prestigeRank: 3,
      teamPrestige: 10,
      primaryColor: 0xc10230,
      secondaryColor: 0xa2a9ad,
      conference: { name: 'Big Ten', enum: 'BigTen', presentationId: 1 },
    },
  );
  assert.ok(Object.keys(ohioState?.data ?? {}).length >= 424);
});

test('exports complete team-scoped players for dynamic roster responses', async () => {
  const catalog = await exportDynastyAssets({ assetRoot, slot: 0 });

  assert.equal(catalog.players.length, 11730);
  assert.ok(
    catalog.players.every(
      (player) =>
        player.firstName &&
        player.lastName &&
        player.position &&
        player.teamIndex >= 0 &&
        player.teamIndex < catalog.teams.length,
    ),
  );
  const ohioState = catalog.players.filter((player) => player.teamIndex === 68);
  assert.equal(ohioState.length, 85);
  const julianSayin = ohioState.find(
    (player) => player.firstName === 'Julian' && player.lastName === 'Sayin',
  );
  assert.deepEqual(
    {
      position: julianSayin?.position,
      jerseyNum: julianSayin?.jerseyNum,
      overallRating: julianSayin?.overallRating,
    },
    { position: 'QB', jerseyNum: 10, overallRating: 94 },
  );
  assert.ok(Object.keys(julianSayin?.ratings ?? {}).length >= 30);
});
