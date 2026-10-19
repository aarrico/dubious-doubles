import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { loadGameSet } from '@pokepc/dataset/lib/fs'

const toolDir = dirname(fileURLToPath(import.meta.url))
const packageDir = resolve(
  dirname(fileURLToPath(import.meta.resolve('@pokepc/dataset/lib/fs'))),
  '../..',
)

export function sha256(bytes) {
  return createHash('sha256').update(bytes).digest('hex')
}

// Object keys and top-level records are sorted; upstream array order is preserved.
export function stableJSON(value) {
  const sortKeys = (entry) => {
    if (Array.isArray(entry)) return entry.map(sortKeys)
    if (entry !== null && typeof entry === 'object') {
      return Object.fromEntries(
        Object.keys(entry).sort().map((key) => [key, sortKeys(entry[key])]),
      )
    }
    return entry
  }
  return `${JSON.stringify(sortKeys(value), null, 2)}\n`
}

export const catalogScopes = Object.freeze({
  pokemon: 'champions',
  moves: 'champions',
  abilities: 'champions',
  items: 'champions',
  'battle-states': 'champions',
  types: 'base',
  natures: 'base',
})

export function catalogPayload(kind, inputRecords, inputText) {
  if (!Object.hasOwn(catalogScopes, kind)) throw new Error(`Unsupported catalog: ${kind}`)
  if (!Array.isArray(inputRecords) || inputRecords.length === 0) {
    throw new Error(`${kind} roster is empty or missing`)
  }
  const records = [...inputRecords]
  const text = {}
  const ids = new Set()
  for (const record of records) {
    if (typeof record?.id !== 'string' || !record.id.trim()) {
      throw new Error(`${kind} record has no id`)
    }
    if (ids.has(record.id)) throw new Error(`Duplicate ${kind} id: ${record.id}`)
    ids.add(record.id)
    const translation = inputText?.[record.id]
    if (typeof translation?.name !== 'string' || !translation.name.trim()) {
      throw new Error(`Missing English name for ${kind}/${record.id}`)
    }
    text[record.id] = translation
  }
  records.sort((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0)
  return { scope: catalogScopes[kind], records, text }
}

export async function exportSnapshot(output) {
  if (process.env.POKEPC_DATASET_DIR) {
    throw new Error('Unset POKEPC_DATASET_DIR: snapshots must use the pinned installed package')
  }
  const ownPackage = JSON.parse(await readFile(join(toolDir, 'package.json'), 'utf8'))
  const upstream = JSON.parse(await readFile(join(packageDir, 'package.json'), 'utf8'))
  if (upstream.name !== '@pokepc/dataset' || upstream.version !== ownPackage.dependencies['@pokepc/dataset']) {
    throw new Error('Installed dataset does not match the exact package.json pin; run pnpm install --frozen-lockfile')
  }
  const lockfile = await readFile(join(toolDir, 'pnpm-lock.yaml'))
  const champions = loadGameSet('champions', {
    dataDir: join(packageDir, 'data'),
    kinds: Object.keys(catalogScopes).filter((kind) => catalogScopes[kind] === 'champions'),
    locales: ['eng'],
  })
  const files = {}
  const counts = {}
  for (const [kind, scope] of Object.entries(catalogScopes)) {
    // Shared reference catalogs have no Champions mod; do not imply availability.
    const records = scope === 'champions'
      ? champions.records[kind]
      : JSON.parse(await readFile(join(packageDir, 'data', `${kind}.json`), 'utf8'))
    const text = scope === 'champions'
      ? champions.text.eng?.[kind]
      : JSON.parse(await readFile(join(packageDir, 'data/i18n/eng', `${kind}.json`), 'utf8'))
    const payload = catalogPayload(kind, records, text)
    files[`${kind}.json`] = stableJSON(payload)
    counts[kind] = payload.records.length
  }
  files.LICENSE = await readFile(join(packageDir, 'LICENSE'))
  const manifest = {
    formatVersion: 2,
    source: {
      package: upstream.name,
      version: upstream.version,
      gameSet: 'champions',
      locale: 'eng',
      lockfileSha256: sha256(lockfile),
    },
    files: Object.fromEntries(Object.entries(files).map(([name, bytes]) => [name, sha256(bytes)])),
  }

  await mkdir(dirname(output), { recursive: true })
  // Never replace an existing snapshot, including a previously failed export.
  await mkdir(output)
  for (const [name, bytes] of Object.entries(files)) {
    await writeFile(join(output, name), bytes, { flag: 'wx' })
  }
  await writeFile(join(output, 'manifest.json'), stableJSON(manifest), { flag: 'wx' })
  return counts
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    if (process.argv.length > 3) throw new Error('Usage: node export.mjs [output-directory]')
    const output = process.argv[2]
      ? resolve(process.argv[2])
      : resolve(toolDir, '../../.cache/pokepc/8.2.0/format-2')
    const counts = await exportSnapshot(output)
    const summary = Object.entries(counts).map(([kind, count]) => `${kind}: ${count}`).join(', ')
    console.log(`Exported snapshot to ${output} (${summary}); no database writes performed.`)
  } catch (error) {
    console.error(`PokéPC export failed: ${error.message}`)
    process.exitCode = 1
  }
}
