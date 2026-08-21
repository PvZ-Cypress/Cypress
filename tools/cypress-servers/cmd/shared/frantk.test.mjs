import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import assert from 'node:assert/strict';

import { assertTableSchemaBound, largestTableByName, openSlotFranchise } from './frantk.mjs';

const assetRoot = 'C:/Users/Shadow/Desktop/CFB27/Release/Dynasty_Assets';
const slot = 0;

/*
 * Regression cover for the FranTk 486 update. madden-franchise only honours
 * schemaFileMap when the picked schema path is XML, so without an explicit
 * schemaOverride it silently fell back to a bundled snapshot. That snapshot
 * predates Coach.LeagueJobMotivation, its attribute count no longer matched the
 * table header, and FranchiseFileTable's schema setter dropped it without
 * raising. Every named Coach read then returned null and the catalog exported
 * 433 blank coaches while still exiting successfully.
 */
test('binds the slot schema to the Coach table instead of a generic fallback', async (t) => {
  if (!fs.existsSync(path.join(assetRoot, String(slot)))) {
    t.skip('local CFB27 Dynasty assets are unavailable');
    return;
  }

  const franchise = await openSlotFranchise({
    file: path.join(assetRoot, String(slot), 'dynasty-dynasty-binary.FTC'),
    assetRoot,
    slot,
  });
  const coachTable = assertTableSchemaBound(largestTableByName(franchise, 'Coach'), 'Coach');

  assert.equal(
    coachTable.schema.attributes.length,
    coachTable.header.numMembers,
    'the schema setter rejects any schema whose attribute count differs from the header',
  );
  assert.ok(
    coachTable.schema.attributes.some((attribute) => attribute.name === 'FirstName'),
    'a generic fallback schema would name every field Field_N',
  );

  await coachTable.readRecords();
  const populated = coachTable.records.filter((record) => !record.isEmpty);
  assert.ok(populated.length > 400, `expected a populated Coach table, got ${populated.length}`);

  const named = populated.filter(
    (record) => String(record.FirstName ?? '') && String(record.LastName ?? ''),
  );
  assert.ok(
    named.length >= populated.length - 4,
    `expected named coaches, got ${named.length} of ${populated.length}`,
  );
});

test('inflates enum attributes so enum-typed fields read back as names', async (t) => {
  if (!fs.existsSync(path.join(assetRoot, String(slot)))) {
    t.skip('local CFB27 Dynasty assets are unavailable');
    return;
  }

  const franchise = await openSlotFranchise({
    file: path.join(assetRoot, String(slot), 'dynasty-dynasty-binary.FTC'),
    assetRoot,
    slot,
  });
  const coachTable = largestTableByName(franchise, 'Coach');
  await coachTable.readRecords();
  const coach = coachTable.records.find((record) => !record.isEmpty && String(record.FirstName));

  // The XML schema branch leaves attribute.enum as a bare name; without the
  // inflation pass Position reads back as its raw ordinal instead of HeadCoach.
  assert.ok(
    ['HeadCoach', 'OffensiveCoordinator', 'DefensiveCoordinator'].includes(String(coach.Position)),
    `expected an enum name for Position, got ${String(coach.Position)}`,
  );
});

test('rejects a table that fell back to a generic schema', () => {
  const generic = {
    header: { numMembers: 3 },
    schema: { attributes: [{ name: 'Field_0' }, { name: 'Field_1' }, { name: 'Field_2' }] },
  };
  assert.throws(() => assertTableSchemaBound(generic, 'Coach'), /did not bind its schema/);
  assert.throws(() => assertTableSchemaBound({ header: { numMembers: 3 } }, 'Coach'), /did not bind its schema/);
});
