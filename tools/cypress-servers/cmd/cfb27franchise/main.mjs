import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import {
  assertTableSchemaBound,
  openSlotFranchise,
  requiredFile,
  requiredLargestTable,
} from '../shared/frantk.mjs';

const teamKeyBase = 830865408;
const zeroReference = '00000000000000000000000000000000';

export async function inspectFranchise({ input, assetRoot, slot = 0 }) {
  const franchise = await openFranchise({ input, assetRoot, slot });
  return summarizeFranchise(franchise, input);
}

export async function initializeFranchise({ input, output, assetRoot, slot = 0 }) {
  assertSeparateOutput(input, output);
  const franchise = await openFranchise({ input, assetRoot, slot });
  const teamTable = requiredLargestTable(franchise, 'Team');
  const coachTable = requiredLargestTable(franchise, 'Coach');
  const userTable = requiredLargestTable(franchise, 'FranchiseUser');
  await Promise.all([teamTable.readRecords(), coachTable.readRecords(), userTable.readRecords()]);

  for (const team of teamTable.records) {
    if (!team.isEmpty && team.getFieldByKey('UserCharacter')) team.UserCharacter = zeroReference;
  }
  for (const coach of coachTable.records) {
    if (!coach.isEmpty && coach.getFieldByKey('IsUserControlled')) coach.IsUserControlled = false;
  }
  for (const user of userTable.records) {
    if (!user.isEmpty && user.getFieldByKey('UserEntity')) user.UserEntity = zeroReference;
  }

  await saveAndValidate(franchise, output);
  return inspectFranchise({ input: output, assetRoot, slot });
}

export async function selectTeam({ input, output, assetRoot, slot = 0, teamKey, coachKey = 0 }) {
  assertSeparateOutput(input, output);
  const franchise = await openFranchise({ input, assetRoot, slot });
  const teamTable = requiredLargestTable(franchise, 'Team');
  const coachTable = requiredLargestTable(franchise, 'Coach');
  const userTable = requiredLargestTable(franchise, 'FranchiseUser');
  await Promise.all([
    teamTable.readRecords(),
    coachTable.readRecords(),
    userTable.readRecords(),
  ]);

  const teamIndex = Number(teamKey) - teamKeyBase;
  const selectedTeam = teamTable.records[teamIndex];
  if (!Number.isInteger(teamIndex) || teamIndex < 0 || !selectedTeam || selectedTeam.isEmpty) {
    throw new Error(`team key ${teamKey} does not resolve to a populated Team record`);
  }
  const requestedAssetId = Number(coachKey) - 547356166;
  const staff = ['HeadCoach', 'OffensiveCoordinator', 'DefensiveCoordinator']
    .map((field) => {
      const reference = selectedTeam.getValueByKey(field);
      const data = selectedTeam.getReferenceDataByKey(field);
      const coach = data?.tableId === coachTable.header.tableId
        ? coachTable.records[data.rowNumber]
        : undefined;
      return { field, reference, coach };
    })
    .filter(({ reference, coach }) => reference && reference !== zeroReference && coach && !coach.isEmpty);
  let selectedStaff = staff.find(({ field }) => field === 'HeadCoach');
  if (Number.isInteger(requestedAssetId) && requestedAssetId > 0) {
    selectedStaff = staff.find(({ coach }) => coachAssetId(coach) === requestedAssetId);
    if (!selectedStaff) {
      throw new Error(`coach key ${coachKey} is not on team key ${teamKey}`);
    }
  }
  if (!selectedStaff) throw new Error(`team key ${teamKey} has no populated coaching staff`);
  const franchiseUser = userTable.records.find((record) => !record.isEmpty);
  if (!franchiseUser) {
    throw new Error('full Dynasty save has no FranchiseUser record');
  }

  for (const team of teamTable.records) {
    if (!team.isEmpty && team.getFieldByKey('UserCharacter')) {
      team.UserCharacter = zeroReference;
    }
  }
  for (const coach of coachTable.records) {
    if (!coach.isEmpty && coach.getFieldByKey('IsUserControlled')) {
      coach.IsUserControlled = false;
    }
  }
  selectedTeam.UserCharacter = selectedStaff.reference;
  selectedStaff.coach.IsUserControlled = true;
  franchiseUser.UserEntity = selectedStaff.reference;

  await saveAndValidate(franchise, output);
  return inspectFranchise({ input: output, assetRoot, slot });
}

export async function advanceFranchise({
  input,
  output,
  assetRoot,
  slot = 0,
  currentWeek,
  stage,
}) {
  assertSeparateOutput(input, output);
  const franchise = await openFranchise({ input, assetRoot, slot });
  const seasonTable = requiredLargestTable(franchise, 'SeasonInfo');
  await seasonTable.readRecords();
  const season = seasonTable.records.find((record) => !record.isEmpty);
  if (!season) throw new Error('full Dynasty save has no SeasonInfo record');

  const normalizedStage = String(stage ?? '').trim().toLowerCase();
  const week = Number(currentWeek);
  if (!/^week_\d+$/.test(normalizedStage) || !Number.isInteger(week) || week < 1) {
    throw new Error(`unsupported persisted Dynasty stage ${stage} / week ${currentWeek}`);
  }
  const stageWeek = Number(normalizedStage.slice('week_'.length));
  if (stageWeek !== week) {
    throw new Error(`Dynasty stage ${stage} does not match week ${currentWeek}`);
  }

  season.CurrentStage = 'NFLSeason';
  season.CurrentWeekType = 'RegularSeason';
  season.CurrentWeek = week;
  if (season.getFieldByKey('IsLeagueStarted')) season.IsLeagueStarted = true;

  await saveAndValidate(franchise, output);
  return inspectFranchise({ input: output, assetRoot, slot });
}

async function openFranchise({ input, assetRoot, slot }) {
  const resolvedInput = requiredFile(input, 'Dynasty input');
  const header = fs.readFileSync(resolvedInput, { encoding: null }).subarray(0, 8).toString();
  if (header !== 'FBCHUNKS') {
    throw new Error(`Dynasty input is not a full FBCHUNKS franchise save: ${resolvedInput}`);
  }
  const franchise = await openSlotFranchise({ file: resolvedInput, assetRoot, slot });
  if (franchise.type?.format !== 'franchise') {
    throw new Error(`expected a full franchise save, got ${franchise.type?.format ?? 'unknown'}`);
  }
  // Team selection writes UserCharacter/IsUserControlled through named fields.
  // A generic fallback schema would make those writes land on the wrong offsets.
  assertTableSchemaBound(requiredLargestTable(franchise, 'Coach'), 'Coach');
  assertTableSchemaBound(requiredLargestTable(franchise, 'Team'), 'Team');
  return franchise;
}

async function summarizeFranchise(franchise, input) {
  const teamTable = requiredLargestTable(franchise, 'Team');
  const coachTable = requiredLargestTable(franchise, 'Coach');
  const userTable = requiredLargestTable(franchise, 'FranchiseUser');
  const seasonTable = requiredLargestTable(franchise, 'SeasonInfo');
  await Promise.all([
    teamTable.readRecords(),
    coachTable.readRecords(),
    userTable.readRecords(),
    seasonTable.readRecords(),
  ]);
  const user = userTable.records.find((record) => !record.isEmpty);
  const season = seasonTable.records.find((record) => !record.isEmpty);
  if (!user || !season) throw new Error('full Dynasty save is missing user or season state');

  const userEntity = user.getReferenceDataByKey('UserEntity');
  const userEntityValue = user.getValueByKey('UserEntity');
  const hasSelectedUserEntity = userEntityValue && userEntityValue !== zeroReference;
  const selectedCoach =
    hasSelectedUserEntity && userEntity?.tableId === coachTable.header.tableId
      ? coachTable.records[userEntity.rowNumber]
      : undefined;
  const selectedTeam = hasSelectedUserEntity ? teamTable.records.find((team) => {
    if (team.isEmpty) return false;
    return (
      team.getValueByKey('UserCharacter') === userEntityValue ||
      team.getValueByKey('HeadCoach') === userEntityValue
    );
  }) : undefined;
  const userControlledCoachCount = coachTable.records.filter(
    (coach) => !coach.isEmpty && Boolean(coach.IsUserControlled),
  ).length;
  const userAssignedTeamCount = teamTable.records.filter(
    (team) => !team.isEmpty && team.getValueByKey('UserCharacter') !== zeroReference,
  ).length;

  return {
    path: path.resolve(input),
    sha256: sha256File(input),
    selectedTeamKey: selectedTeam ? teamKeyBase + selectedTeam.index : 0,
    selectedTeamRecord: selectedTeam?.index ?? -1,
    selectedTeamIndex: selectedTeam ? numberValue(selectedTeam.TeamIndex) : -1,
    selectedTeam: selectedTeam
      ? {
          displayName: stringValue(selectedTeam.DisplayName),
          longName: stringValue(selectedTeam.LongName),
          nickname: stringValue(selectedTeam.NickName),
          presentationId: numberValue(selectedTeam.PresentationId),
        }
      : null,
    selectedCoach: selectedCoach
      ? {
          record: selectedCoach.index,
          firstName: stringValue(selectedCoach.FirstName),
          lastName: stringValue(selectedCoach.LastName),
          position: stringValue(selectedCoach.Position),
          portrait: numberValue(selectedCoach.Portrait),
          level: numberValue(selectedCoach.Level),
        }
      : null,
    userEntityMatchesHeadCoach:
      Boolean(selectedTeam) && selectedTeam.getValueByKey('HeadCoach') === userEntityValue,
    teamUserCharacterMatchesHeadCoach:
      Boolean(selectedTeam) &&
      selectedTeam.getValueByKey('UserCharacter') === selectedTeam.getValueByKey('HeadCoach'),
    userControlledCoachCount,
    userAssignedTeamCount,
    currentStage: stringValue(season.CurrentStage),
    currentWeekType: stringValue(season.CurrentWeekType),
    currentWeek: numberValue(season.CurrentWeek),
    currentSeasonYear: numberValue(season.CurrentSeasonYear),
  };
}

async function saveAndValidate(franchise, output) {
  const resolvedOutput = path.resolve(output);
  fs.mkdirSync(path.dirname(resolvedOutput), { recursive: true });
  await franchise.save(resolvedOutput);
  const header = fs.readFileSync(resolvedOutput, { encoding: null }).subarray(0, 8).toString();
  if (header !== 'FBCHUNKS') {
    throw new Error(`saved Dynasty artifact is not FBCHUNKS: ${resolvedOutput}`);
  }
}

function assertSeparateOutput(input, output) {
  if (!output) throw new Error('output path is required');
  if (path.resolve(input).toLowerCase() === path.resolve(output).toLowerCase()) {
    throw new Error('input and output paths must differ for atomic Dynasty mutation');
  }
}

function sha256File(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

function stringValue(value) {
  return value == null ? '' : String(value);
}

function numberValue(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function coachAssetId(coach) {
  const match = /_(\d+)$/.exec(stringValue(coach.AssetName));
  return match ? Number(match[1]) : 0;
}

function parseCLI(argv) {
  const command = argv[0];
  const values = {};
  for (let index = 1; index < argv.length; index += 2) {
    const flag = argv[index];
    if (!flag?.startsWith('--') || index + 1 >= argv.length) {
      throw new Error(`invalid argument ${flag ?? ''}`.trim());
    }
    values[flag.slice(2)] = argv[index + 1];
  }
  const common = {
    input: values.input,
    output: values.output,
    assetRoot: values['asset-root'],
    slot: Number(values.slot ?? 0),
  };
  if (command === 'inspect') return { command, args: common };
  if (command === 'initialize') return { command, args: common };
  if (command === 'select-team') {
    return {
      command,
      args: {
        ...common,
        teamKey: Number(values['team-key']),
        coachKey: Number(values['coach-key'] ?? 0),
      },
    };
  }
  if (command === 'advance') {
    return {
      command,
      args: {
        ...common,
        currentWeek: Number(values['current-week']),
        stage: values.stage,
      },
    };
  }
  throw new Error(`unknown command ${command ?? ''}`.trim());
}

async function main() {
  const { command, args } = parseCLI(process.argv.slice(2));
  const result =
    command === 'inspect'
      ? await inspectFranchise(args)
      : command === 'initialize'
        ? await initializeFranchise(args)
      : command === 'select-team'
        ? await selectTeam(args)
        : await advanceFranchise(args);
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? '').href) {
  main().catch((error) => {
    process.stderr.write(`${error?.stack ?? error}\n`);
    process.exitCode = 1;
  });
}
