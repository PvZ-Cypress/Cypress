import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import assert from 'node:assert/strict';

import {
  advanceFranchise,
  initializeFranchise,
  inspectFranchise,
  selectTeam,
} from './main.mjs';

const assetRoot = 'C:/Users/Shadow/Desktop/CFB27/Release/Dynasty_Assets';

// The saves directory accumulates Dynasty files from older game builds, and a
// save only parses against the schema revision it was written with. Mirror the
// launcher's Find-CFB27DynastySeed and take the newest FBCHUNKS save so these
// tests follow whichever build Dynasty_Assets currently holds.
const source = newestDynastySave(
  'C:/Users/Shadow/Documents/EA SPORTS College Football 27/Saves',
);

test('initializes a clean full Dynasty without a selected team or coach', async (t) => {
  if (!source || !fs.existsSync(path.join(assetRoot, '0'))) {
    t.skip('local CFB27 Dynasty seed/assets are unavailable');
    return;
  }

  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'cfb27-franchise-init-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const initializedPath = path.join(directory, 'initialized.DYNASTY');
  const sourceHash = sha256(source);

  const initialized = await initializeFranchise({
    input: source,
    output: initializedPath,
    assetRoot,
    slot: 0,
  });

  assert.equal(initialized.selectedTeamKey, 0);
  assert.equal(initialized.selectedTeam, null);
  assert.equal(initialized.selectedCoach, null);
  assert.equal(initialized.userControlledCoachCount, 0);
  assert.equal(initialized.userAssignedTeamCount, 0);
  assert.equal(fs.readFileSync(initializedPath, { encoding: null }).subarray(0, 8).toString(), 'FBCHUNKS');
  assert.equal(sha256(source), sourceHash, 'initialization must never overwrite the seed');
});

test('selects Ohio State in a copied full Dynasty and persists advancement', async (t) => {
  if (!source || !fs.existsSync(path.join(assetRoot, '0'))) {
    t.skip('local CFB27 Dynasty seed/assets are unavailable');
    return;
  }

  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'cfb27-franchise-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const selectedPath = path.join(directory, 'selected.DYNASTY');
  const advancedPath = path.join(directory, 'advanced.DYNASTY');
  const sourceHash = sha256(source);

  const selected = await selectTeam({
    input: source,
    output: selectedPath,
    assetRoot,
    slot: 0,
    teamKey: 830865495,
  });
  assert.equal(selected.selectedTeamKey, 830865495);
  assert.equal(selected.selectedTeamIndex, 68);
  assert.equal(selected.selectedCoach.firstName, 'Ryan');
  assert.equal(selected.selectedCoach.lastName, 'Day');
  assert.equal(selected.selectedCoach.position, 'HeadCoach');
  assert.equal(selected.userEntityMatchesHeadCoach, true);
  assert.equal(selected.teamUserCharacterMatchesHeadCoach, true);
  assert.equal(fs.readFileSync(selectedPath, { encoding: null }).subarray(0, 8).toString(), 'FBCHUNKS');
  assert.equal(sha256(source), sourceHash, 'the supplied offline save must never be overwritten');

  const selectedCoordinatorPath = path.join(directory, 'selected-coordinator.DYNASTY');
  const selectedCoordinator = await selectTeam({
    input: source,
    output: selectedCoordinatorPath,
    assetRoot,
    slot: 0,
    teamKey: 830865495,
    coachKey: 547357043,
  });
  assert.equal(selectedCoordinator.selectedTeamKey, 830865495);
  assert.equal(selectedCoordinator.selectedCoach.firstName, 'Arthur');
  assert.equal(selectedCoordinator.selectedCoach.lastName, 'Smith');
  assert.equal(selectedCoordinator.selectedCoach.position, 'OffensiveCoordinator');
  assert.equal(selectedCoordinator.userEntityMatchesHeadCoach, false);
  assert.equal(selectedCoordinator.teamUserCharacterMatchesHeadCoach, false);

  const advanced = await advanceFranchise({
    input: selectedPath,
    output: advancedPath,
    assetRoot,
    slot: 0,
    currentWeek: 1,
    stage: 'week_1',
  });
  assert.equal(advanced.currentStage, 'NFLSeason');
  assert.equal(advanced.currentWeekType, 'RegularSeason');
  assert.equal(advanced.currentWeek, 1);

  const reopened = await inspectFranchise({
    input: advancedPath,
    assetRoot,
    slot: 0,
  });
  assert.equal(reopened.selectedTeamKey, 830865495);
  assert.equal(reopened.selectedCoach.lastName, 'Day');
  assert.equal(reopened.currentStage, 'NFLSeason');
  assert.equal(reopened.currentWeekType, 'RegularSeason');
  assert.equal(reopened.currentWeek, 1);
});

function sha256(file) {
  return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

function newestDynastySave(savesDirectory) {
  if (!fs.existsSync(savesDirectory)) return null;
  const candidates = fs
    .readdirSync(savesDirectory, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.toUpperCase().startsWith('DYNASTY'))
    .map((entry) => path.join(savesDirectory, entry.name))
    .filter(
      (file) =>
        fs.readFileSync(file, { encoding: null }).subarray(0, 8).toString() === 'FBCHUNKS',
    )
    .sort((left, right) => fs.statSync(right).mtimeMs - fs.statSync(left).mtimeMs);
  return candidates[0] ?? null;
}
