import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import { assertTableSchemaBound, largestTableByName, openSlotFranchise } from '../shared/frantk.mjs';

const catalogVersion = 1;

export async function exportDynastyAssets({ assetRoot, slot }) {
  const slotRoot = path.resolve(assetRoot, String(slot));
  const mainSchema = path.join(slotRoot, 'franchise-schemas.FTX');
  const coachSchema = path.join(slotRoot, 'franchise-schemas', 'coach.FTX');
  const teamSchema = path.join(slotRoot, 'franchise-schemas', 'team.FTX');
  const playerSchema = path.join(slotRoot, 'franchise-schemas', 'player.FTX');
  const pipelineSchema = path.join(slotRoot, 'franchise-schemas', 'pipeline.FTX');
  const archetypeSchema = path.join(
    slotRoot,
    'franchise-schemas',
    'coachtalentarchetype.FTX',
  );
  const coachPositionSchema = path.join(
    slotRoot,
    'franchise-schemas',
    'coachposition.FTX',
  );
  const baseSchemeSchema = path.join(
    slotRoot,
    'franchise-schemas',
    'basescheme.FTX',
  );
  const letterGradeSchema = path.join(
    slotRoot,
    'football-schemas',
    'lettergrade.FTX',
  );
  const positionSchema = path.join(
    slotRoot,
    'football-schemas',
    'positione.FTX',
  );
  const dynastyFile = path.join(slotRoot, 'dynasty-dynasty-binary.FTC');
  for (const required of [
    mainSchema,
    coachSchema,
    teamSchema,
    playerSchema,
    pipelineSchema,
    archetypeSchema,
    coachPositionSchema,
    baseSchemeSchema,
    letterGradeSchema,
    positionSchema,
    dynastyFile,
  ]) {
    if (!fs.statSync(required, { throwIfNoEntry: false })?.isFile()) {
      throw new Error(`missing CFB27 dynasty asset: ${required}`);
    }
  }

  const franchise = await openSlotFranchise({ file: dynastyFile, assetRoot, slot });
  const coachTable = largestTableByName(franchise, 'Coach');
  if (!coachTable) {
    throw new Error('Coach table is missing from dynasty FTC');
  }
  const teamTable = largestTableByName(franchise, 'Team');
  if (!teamTable || teamTable.header.recordCapacity < 100) {
    throw new Error('primary Team table is missing from dynasty FTC');
  }
  const playerTable = largestTableByName(franchise, 'Player');
  if (!playerTable || playerTable.header.recordCapacity < 10000) {
    throw new Error('primary Player table is missing from dynasty FTC');
  }
  for (const [table, name] of [
    [coachTable, 'Coach'],
    [teamTable, 'Team'],
    [playerTable, 'Player'],
  ]) {
    assertTableSchemaBound(table, name);
  }
  await Promise.all([
    coachTable.readRecords(),
    teamTable.readRecords(),
    playerTable.readRecords(),
  ]);

  const coachSchemaText = fs.readFileSync(coachSchema, 'utf8');
  const teamSchemaText = fs.readFileSync(teamSchema, 'utf8');
  const coachFieldNames = schemaFieldNames(coachSchemaText);
  const teamFieldNames = schemaFieldNames(teamSchemaText);
  if (coachFieldNames.length < 137) {
    throw new Error(
      `Coach schema has ${coachFieldNames.length} fields; expected at least 137`,
    );
  }
  if (teamFieldNames.length < 424) {
    throw new Error(
      `Team schema has ${teamFieldNames.length} fields; expected at least 424`,
    );
  }

  const enums = {
    pipeline: enumValues(fs.readFileSync(pipelineSchema, 'utf8'), 'Pipeline'),
    archetype: enumValues(
      fs.readFileSync(archetypeSchema, 'utf8'),
      'CoachTalentArcheType',
    ),
    coachPosition: enumValues(
      fs.readFileSync(coachPositionSchema, 'utf8'),
      'CoachPosition',
    ),
    baseScheme: enumValues(
      fs.readFileSync(baseSchemeSchema, 'utf8'),
      'BaseScheme',
    ),
    letterGrade: enumValues(
      fs.readFileSync(letterGradeSchema, 'utf8'),
      'LetterGrade',
    ),
    position: enumValues(
      fs.readFileSync(positionSchema, 'utf8'),
      'PositionE',
    ),
  };
  const [conferenceByTeamRecord, characterVisualsByCoachRecord] =
    await Promise.all([
      resolveConferences(franchise, teamTable),
      resolveCharacterVisuals(franchise, coachTable),
    ]);

  const coaches = coachTable.records
    .filter((record) => !record.isEmpty)
    .map((record) =>
      serializeCoach(
        record,
        coachFieldNames,
        enums,
        characterVisualsByCoachRecord.get(record.index),
      ),
    );
  const teams = teamTable.records
    .filter((record) => !record.isEmpty)
    .map((record) =>
      serializeTeam(
        record,
        teamFieldNames,
        enums,
        conferenceByTeamRecord.get(record.index),
      ),
    );
  const players = playerTable.records
    .filter(
      (record) =>
        !record.isEmpty &&
        stringValue(record.FirstName) &&
        stringValue(record.LastName) &&
        stringValue(record.Position) &&
        numberValue(record.TeamIndex) >= 0 &&
        numberValue(record.TeamIndex) < teams.length,
    )
    .map((record) => serializePlayer(record, enums));

  return {
    version: catalogVersion,
    source: {
      assetRoot: path.resolve(assetRoot),
      slot: Number(slot),
      dataRevisionVersion: integerAttribute(coachSchemaText, 'dataRevisionVersion'),
      dynastySha256: sha256File(dynastyFile),
      coachSchemaSha256: sha256File(coachSchema),
      teamSchemaSha256: sha256File(teamSchema),
      playerSchemaSha256: sha256File(playerSchema),
    },
    coaches,
    teams,
    players,
  };
}

const playerRatingFields = {
  PACC: 'AccelerationRating',
  PAGI: 'AgilityRating',
  PAWR: 'AwarenessRating',
  PBCV: 'BCVisionRating',
  PBSG: 'BlockSheddingRating',
  PBSK: 'BreakSackRating',
  PBTK: 'BreakTackleRating',
  PCAR: 'CarryingRating',
  PCTH: 'CatchingRating',
  PFMS: 'FinesseMovesRating',
  PINJ: 'InjuryRating',
  PJMP: 'JumpingRating',
  PKAC: 'KickAccuracyRating',
  PKPR: 'KickPowerRating',
  PKRT: 'KickReturnRating',
  PLBK: 'LeadBlockRating',
  PLCI: 'CatchInTrafficRating',
  PLHT: 'HitPowerRating',
  PLIB: 'ImpactBlockingRating',
  PLJM: 'JukeMoveRating',
  PLMC: 'ManCoverageRating',
  PLNG: 'LongSnapRating',
  PLPE: 'PressRating',
  PLPM: 'PowerMovesRating',
  PLPR: 'PlayActionRating',
  PLPU: 'PursuitRating',
  PLRL: 'ReleaseRating',
  PLSA: 'ShortThrowAccuracyRating',
  PLSC: 'SpectacularCatchRating',
  PLSM: 'ShortRouteRunningRating',
  PLTR: 'TruckRating',
  PLZC: 'ZoneCoverageRating',
  PPBF: 'PassBlockFinesseRating',
  PPBK: 'PassBlockRating',
  PPBP: 'PassBlockPowerRating',
  PPLA: 'PlayActionRating',
  PRBF: 'RunBlockFinesseRating',
  PRBK: 'RunBlockRating',
  PRBP: 'RunBlockPowerRating',
  PRRD: 'DeepRouteRunningRating',
  PRRM: 'MediumRouteRunningRating',
  PRRS: 'ShortRouteRunningRating',
  PSPD: 'SpeedRating',
  PSTA: 'StaminaRating',
  PSTR: 'StrengthRating',
  PTAD: 'ThrowAccuracyDeepRating',
  PTAK: 'TackleRating',
  PTAM: 'ThrowAccuracyMidRating',
  PTAS: 'ThrowAccuracyShortRating',
  PTGH: 'ToughnessRating',
  PTHA: 'ThrowAccuracyRating',
  PTHP: 'ThrowPowerRating',
  PTOR: 'ThrowOnTheRunRating',
  PTUP: 'ThrowUnderPressureRating',
};

function serializePlayer(record, enums) {
  return {
    id: record.index,
    firstName: stringValue(record.FirstName),
    lastName: stringValue(record.LastName),
    teamIndex: numberValue(record.TeamIndex),
    position: stringValue(record.Position),
    positionValue: enumValue(enums.position, record.Position),
    jerseyNum: numberValue(record.JerseyNum),
    overallRating: numberValue(record.OverallRating),
    portrait: numberValue(record.PLYR_PORTRAIT),
    presentationId: numberValue(record.PresentationId),
    genericHead: numberValue(record.PLYR_GENERICHEAD),
    height: numberValue(record.Height),
    weight: numberValue(record.Weight),
    classYear: numberValue(record.ClassYear),
    developmentTrait: numberValue(record.TraitDevelopment),
    hometown: stringValue(record.PLYR_HOME_TOWN),
    ratings: Object.fromEntries(
      Object.entries(playerRatingFields).map(([tag, field]) => [
        tag,
        numberValue(record[field]),
      ]),
    ),
  };
}

function schemaFieldNames(schemaText) {
  return [...schemaText.matchAll(/<attribute\s+name="([^"]+)"/g)].map(
    (match) => match[1],
  );
}

function enumValues(schemaText, enumName) {
  const escaped = enumName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const body = schemaText.match(
    new RegExp(`<enum\\s+name="${escaped}"[^>]*>([\\s\\S]*?)<\\/enum>`),
  )?.[1];
  if (!body) throw new Error(`enum ${enumName} is missing from its FTX schema`);
  return new Map(
    [...body.matchAll(/<attribute\s+name="([^"]+)"[^>]*\svalue="(-?\d+)"/g)].map(
      (match) => [match[1], Number(match[2])],
    ),
  );
}

async function resolveConferences(franchise, teamTable) {
  const conferenceTable = largestTableByName(franchise, 'Conference');
  const result = new Map();
  if (!conferenceTable) return result;
  await conferenceTable.readRecords();
  const loadedArrays = new Set();
  for (const conference of conferenceTable.records.filter(
    (record) => !record.isEmpty && stringValue(record.Name),
  )) {
    const slotsReference = conference.getReferenceDataByKey('TeamSlots');
    if (!slotsReference) continue;
    const slotsTable = franchise.getTableById(slotsReference.tableId);
    if (!slotsTable) continue;
    if (!loadedArrays.has(slotsTable.header.tableId)) {
      await slotsTable.readRecords();
      loadedArrays.add(slotsTable.header.tableId);
    }
    const slots = slotsTable.records[slotsReference.rowNumber];
    if (!slots || slots.isEmpty) continue;
    const summary = {
      name: stringValue(conference.Name),
      enum: stringValue(conference.ConferenceEnum),
      presentationId: numberValue(conference.PresentationId),
    };
    for (const field of slots.fieldsArray.slice(0, slots.arraySize)) {
      const teamReference = field.referenceData;
      if (teamReference?.tableId === teamTable.header.tableId) {
        result.set(teamReference.rowNumber, summary);
      }
    }
  }
  return result;
}

async function resolveCharacterVisuals(franchise, coachTable) {
  const result = new Map();
  const visualTable = largestTableByName(franchise, 'CharacterVisuals');
  if (!visualTable) return result;
  await visualTable.readRecords();
  for (const coach of coachTable.records.filter((record) => !record.isEmpty)) {
    const reference = coach.getReferenceDataByKey('CharacterVisuals');
    if (!reference || reference.tableId !== visualTable.header.tableId) continue;
    const visual = visualTable.records[reference.rowNumber];
    if (!visual || visual.isEmpty) continue;
    result.set(coach.index, {
      tableId: reference.tableId,
      rowNumber: reference.rowNumber,
      rawData: stringValue(visual.RawData),
    });
  }
  return result;
}

function serializeCoach(record, fieldNames, enums, characterVisuals) {
  const data = Object.fromEntries(
    fieldNames.map((name) => [name, jsonValue(record[name])]),
  );
  return {
    id: record.index,
    firstName: stringValue(record.FirstName),
    lastName: stringValue(record.LastName),
    assetName: stringValue(record.GenericHeadAssetName || record.AssetName),
    portrait: numberValue(record.Portrait),
    level: numberValue(record.Level),
    position: stringValue(record.Position),
    positionValue: enumValue(enums.coachPosition, record.Position),
    teamIndex: numberValue(record.TeamIndex),
    pipeline: stringValue(record.PrimaryPipeline),
    pipelineValue: enumValue(enums.pipeline, record.PrimaryPipeline),
    archetype: stringValue(record.DominantArchetype),
    archetypeValue: enumValue(enums.archetype, record.DominantArchetype),
    coachPrestige: stringValue(record.CoachPrestige),
    coachPrestigeValue: enumValue(enums.letterGrade, record.CoachPrestige),
    characterVisuals: characterVisuals ?? null,
    offensiveScheme: jsonValue(record.OffensiveScheme),
    defensiveScheme: jsonValue(record.DefensiveScheme),
    data,
  };
}

function serializeTeam(record, fieldNames, enums, conference) {
  const data = Object.fromEntries(
    fieldNames.map((name) => [name, jsonValue(record[name])]),
  );
  const offensiveScheme = stringValue(record.CurrentOffensiveScheme);
  const defensiveScheme = stringValue(record.CurrentDefensiveScheme);
  return {
    id: record.index,
    teamKey: 830865408 + record.index,
    teamIndex: numberValue(record.TeamIndex),
    presentationId: numberValue(record.PresentationId),
    assetName: stringValue(record.AssetName),
    displayName: stringValue(record.DisplayName),
    longName: stringValue(record.LongName),
    nickname: stringValue(record.NickName),
    nicknameAlt: stringValue(record.NickNameAlt),
    shortName: stringValue(record.ShortName),
    logo: numberValue(record.TEAM_LOGO),
    defensiveRating: numberValue(record.TEAM_RATINGDEF),
    offensiveRating: numberValue(record.TEAM_RATINGOFF),
    overallRating: numberValue(record.TEAM_RATINGOVR),
    offensiveScheme,
    offensiveSchemeValue: enumValue(enums.baseScheme, offensiveScheme),
    defensiveScheme,
    defensiveSchemeValue: enumValue(enums.baseScheme, defensiveScheme),
    prestigeRank: numberValue(record.PrestigeRank),
    teamPrestige: numberValue(record.TeamPrestige),
    primaryColor: packedRGB(
      record.TEAM_BACKGROUNDCOLORR,
      record.TEAM_BACKGROUNDCOLORG,
      record.TEAM_BACKGROUNDCOLORB,
    ),
    secondaryColor: packedRGB(
      record.TEAM_BACKGROUNDCOLORR2,
      record.TEAM_BACKGROUNDCOLORG2,
      record.TEAM_BACKGROUNDCOLORB2,
    ),
    conference: conference ?? null,
    data,
  };
}

function enumValue(values, name) {
  const value = values.get(stringValue(name));
  return Number.isInteger(value) ? value : -1;
}

function packedRGB(red, green, blue) {
  return (numberValue(red) << 16) | (numberValue(green) << 8) | numberValue(blue);
}

function jsonValue(value, seen = new WeakSet()) {
  if (value == null || ['string', 'number', 'boolean'].includes(typeof value)) {
    return value ?? null;
  }
  if (typeof value === 'bigint') return value.toString();
  if (Buffer.isBuffer(value)) return { base64: value.toString('base64') };
  if (Array.isArray(value)) return value.map((item) => jsonValue(item, seen));
  if (typeof value !== 'object') return String(value);
  if (seen.has(value)) return null;
  seen.add(value);
  if (typeof value.toJSON === 'function') {
    return jsonValue(value.toJSON(), seen);
  }
  const result = {};
  for (const [key, item] of Object.entries(value)) {
    if (key.startsWith('_') || key === 'parent') continue;
    result[key] = jsonValue(item, seen);
  }
  return result;
}

function stringValue(value) {
  return value == null ? '' : String(value);
}

function numberValue(value) {
  const result = Number(value);
  return Number.isFinite(result) ? result : 0;
}

function integerAttribute(xml, name) {
  const match = xml.match(new RegExp(`${name}="(-?\\d+)"`));
  return match ? Number(match[1]) : 0;
}

function sha256File(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

async function writeCatalog(output, catalog) {
  const destination = path.resolve(output);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  const temporary = `${destination}.${process.pid}.tmp`;
  try {
    fs.writeFileSync(temporary, `${JSON.stringify(catalog)}\n`, { flag: 'wx' });
    fs.renameSync(temporary, destination);
  } catch (error) {
    fs.rmSync(temporary, { force: true });
    throw error;
  }
}

function parseArguments(args) {
  const options = { slot: 0 };
  for (let index = 0; index < args.length; index += 2) {
    const key = args[index];
    const value = args[index + 1];
    if (!value) throw new Error(`missing value for ${key}`);
    if (key === '--asset-root') options.assetRoot = value;
    else if (key === '--slot') options.slot = Number(value);
    else if (key === '--output') options.output = value;
    else throw new Error(`unknown argument ${key}`);
  }
  if (!options.assetRoot || !options.output || !Number.isInteger(options.slot)) {
    throw new Error('usage: node main.mjs --asset-root <dir> --slot <n> --output <json>');
  }
  return options;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const options = parseArguments(process.argv.slice(2));
    const catalog = await exportDynastyAssets(options);
    await writeCatalog(options.output, catalog);
    process.stdout.write(
      `exported ${catalog.coaches.length} coaches, ${catalog.teams.length} teams, and ${catalog.players.length} players from slot ${catalog.source.slot} to ${path.resolve(options.output)}\n`,
    );
  } catch (error) {
    process.stderr.write(`${error?.stack || error}\n`);
    process.exitCode = 1;
  }
}
