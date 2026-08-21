import fs from 'node:fs';
import path from 'node:path';

import Franchise, { FranchiseEnum } from 'madden-franchise';

/*
 * madden-franchise resolves its schema through `schemaPicker`, which returns a
 * bundled `.gz` snapshot keyed off the franchise file's embedded schema version.
 * `FranchiseSchema.evaluate()` dispatches on that path's extension, and only the
 * `.ftx`/`.xml` branch consults `schemaFileMap`. Passing `schemaFileMap` alone
 * therefore has no effect: the bundled snapshot wins and the Dynasty_Assets
 * schema files are never read.
 *
 * That went unnoticed while the bundled snapshot happened to agree with the
 * shipped tables. The FranTk 486 update added `Coach.LeagueJobMotivation`,
 * taking the Coach table's `numMembers` from 137 to 138. `FranchiseFileTable`'s
 * schema setter silently returns when `schema.attributes.length` disagrees with
 * `header.numMembers`, so the Coach table fell back to a generic header-derived
 * schema whose fields are named `Field_0..Field_137`. Every named read then
 * returned null and the catalog exported 433 blank coaches without erroring.
 *
 * `schemaOverride.path` is consumed verbatim by `FranchiseFile.parse()`, so
 * pointing it at the slot's own `franchise-schemas.FTX` forces the XML branch
 * and makes the on-disk schema authoritative for whichever build is installed.
 */
export async function openSlotFranchise({ file, assetRoot, slot = 0 }) {
  const slotRoot = requiredDirectory(path.join(assetRoot, String(slot)), 'Dynasty asset slot');
  const mainSchema = requiredFile(
    path.join(slotRoot, 'franchise-schemas.FTX'),
    'main franchise schema',
  );
  const franchise = await Franchise.create(path.resolve(file), {
    autoParse: true,
    gameTypeOverride: 'college',
    gameYearOverride: 27,
    schemaFileMap: buildSchemaFileMap(slotRoot, mainSchema),
    useNewSchemaGeneration: true,
    schemaOverride: { path: mainSchema },
  });
  resolveSchemaEnums(franchise);
  return franchise;
}

/*
 * The `.gz` branch inflates each attribute's enum name into a `FranchiseEnum`
 * before handing the schema to the tables; the XML branch leaves the bare name
 * behind. `FranchiseFileField` calls `offset.enum.getMemberByValue(...)`, so
 * without this pass an enum-typed field reads back as its raw integer and
 * `Coach.Position` becomes `0` instead of `HeadCoach`.
 *
 * Tables already hold a reference to these schema objects and records have not
 * been read yet, so mutating the attributes in place is enough.
 */
export function resolveSchemaEnums(franchise) {
  const enums = new Map();
  for (const value of franchise.schemaList?.enums ?? []) {
    if (value?.name) enums.set(value.name, value);
  }
  let resolved = 0;
  for (const schema of franchise.schemaList?.schemas ?? []) {
    for (const attribute of schema.attributes ?? []) {
      if (typeof attribute.enum !== 'string') continue;
      const value = enums.get(attribute.enum);
      if (!value) continue;
      attribute.enum = value instanceof FranchiseEnum ? value : new FranchiseEnum(value);
      resolved += 1;
    }
  }
  return resolved;
}

/*
 * A table whose schema was rejected still reads, but every named field comes
 * back null. Callers depend on named reads, so treat the generic fallback as a
 * hard failure rather than letting it surface as empty records downstream.
 */
export function assertTableSchemaBound(table, name) {
  const attributes = table.schema?.attributes ?? [];
  const isGeneric = attributes.length > 0 && attributes.every((a) => /^Field_\d+$/.test(a.name));
  if (!table.schema || isGeneric) {
    throw new Error(
      `${name} table did not bind its schema (header numMembers=${table.header?.numMembers}, ` +
        `schema attributes=${attributes.length}). Named fields would read as null.`,
    );
  }
  return table;
}

export function largestTableByName(franchise, name) {
  return franchise
    .getAllTablesByName(name)
    .sort((left, right) => right.header.recordCapacity - left.header.recordCapacity)[0];
}

export function requiredLargestTable(franchise, name) {
  const table = largestTableByName(franchise, name);
  if (!table) throw new Error(`${name} table is missing from full Dynasty save`);
  return table;
}

export function buildSchemaFileMap(slotRoot, mainSchema) {
  const result = { main: mainSchema };
  for (const file of walk(slotRoot)) {
    if (!file.toLowerCase().endsWith('.ftx')) continue;
    const relative = path
      .relative(slotRoot, file)
      .replace(/\.ftx$/i, '')
      .replaceAll('/', '\\');
    result[relative] = file;
    result[relative.toLowerCase()] = file;
  }
  return result;
}

export function* walk(root) {
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const file = path.join(root, entry.name);
    if (entry.isDirectory()) yield* walk(file);
    else yield file;
  }
}

export function requiredFile(file, label) {
  const resolved = path.resolve(String(file ?? ''));
  if (!fs.statSync(resolved, { throwIfNoEntry: false })?.isFile()) {
    throw new Error(`${label} is missing: ${resolved}`);
  }
  return resolved;
}

export function requiredDirectory(directory, label) {
  const resolved = path.resolve(String(directory ?? ''));
  if (!fs.statSync(resolved, { throwIfNoEntry: false })?.isDirectory()) {
    throw new Error(`${label} is missing: ${resolved}`);
  }
  return resolved;
}
