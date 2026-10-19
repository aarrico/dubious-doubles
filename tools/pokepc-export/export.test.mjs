import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { loadGameSet } from '@pokepc/dataset/lib/fs'
import { catalogPayload, catalogScopes, exportSnapshot, sha256, stableJSON } from './export.mjs'

const packageDir = resolve(
  dirname(fileURLToPath(import.meta.resolve('@pokepc/dataset/lib/fs'))), '../..',
)

const records = [{ id: 'b' }, { id: 'a' }]
const text = { a: { name: 'A' }, b: { name: 'B' }, extra: { name: 'Not in roster' } }

test('stable JSON sorts keys recursively but preserves array order', () => {
  assert.equal(stableJSON({ z: [2, 1], a: { z: 1, a: 2 } }),
    stableJSON({ a: { a: 2, z: 1 }, z: [2, 1] }))
  assert.deepEqual(JSON.parse(stableJSON({ z: [2, 1] })).z, [2, 1])
})

for (const [kind, scope] of Object.entries(catalogScopes)) {
  test(`${kind}: payload sorts records, labels scope, and includes only roster text`, () => {
    const payload = catalogPayload(kind, records, text)
    assert.equal(payload.scope, scope)
    assert.deepEqual(payload.records.map((record) => record.id), ['a', 'b'])
    assert.equal(payload.text.extra, undefined)
    assert.deepEqual(records.map((record) => record.id), ['b', 'a'])
  })

  test(`${kind}: rejects empty rosters, missing IDs, duplicates, and missing names`, () => {
    assert.throws(() => catalogPayload(kind, [], text), /roster is empty/)
    assert.throws(() => catalogPayload(kind, undefined, text), /roster is empty/)
    assert.throws(() => catalogPayload(kind, [null], text), /no id/)
    assert.throws(() => catalogPayload(kind, [{}], text), /no id/)
    assert.throws(() => catalogPayload(kind, [{ id: 'a' }, { id: 'a' }], text), /Duplicate/)
    assert.throws(() => catalogPayload(kind, [{ id: 'unknown' }], text), /Missing English name/)
    assert.throws(() => catalogPayload(kind, [{ id: 'a' }], { a: { name: ' ' } }), /Missing English name/)
  })
}

test('payload preserves availability flags, relationships, arrays, and descriptions', () => {
  const input = [{ id: 'a', usable: false, learnset: ['z', 'a'], baseForms: ['base'] }]
  const inputText = { a: { name: 'A', desc: 'Line one\nLine two', shortDesc: 'Short' } }
  const payload = catalogPayload('moves', input, inputText)
  assert.deepEqual(payload.records, input)
  assert.deepEqual(payload.text, inputText)
  assert.throws(() => catalogPayload('unrecognized', input, inputText), /Unsupported catalog/)
})

test('installed-package exports include all catalogs, are deterministic, hashed, and never overwritten', async (t) => {
  const root = await mkdtemp(join(tmpdir(), 'pokepc-export-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const first = join(root, 'first')
  const second = join(root, 'second')
  const counts = await exportSnapshot(first)
  assert.deepEqual(await exportSnapshot(second), counts)
  const manifest = JSON.parse(await readFile(join(first, 'manifest.json'), 'utf8'))
  assert.equal(manifest.formatVersion, 2)
  assert.equal(manifest.source.version, '8.2.0')
  assert.equal(manifest.source.gameSet, 'champions')
  assert.match(manifest.source.lockfileSha256, /^[a-f0-9]{64}$/)
  const expectedFiles = [...Object.keys(catalogScopes).map((kind) => `${kind}.json`), 'LICENSE']
  assert.deepEqual(Object.keys(manifest.files).sort(), expectedFiles.sort())
  for (const name of [...expectedFiles, 'manifest.json']) {
    assert.deepEqual(await readFile(join(first, name)), await readFile(join(second, name)))
  }
  for (const [name, hash] of Object.entries(manifest.files)) {
    assert.equal(sha256(await readFile(join(first, name))), hash)
  }

  const champions = loadGameSet('champions', { dataDir: join(packageDir, 'data'), locales: ['eng'] })
  const payloads = {}
  for (const [kind, scope] of Object.entries(catalogScopes)) {
    const payload = JSON.parse(await readFile(join(first, `${kind}.json`), 'utf8'))
    payloads[kind] = payload
    assert.equal(payload.scope, scope)
    assert.ok(counts[kind] > 0)
    assert.equal(payload.records.length, counts[kind])
    const upstreamRecords = scope === 'champions'
      ? champions.records[kind]
      : JSON.parse(await readFile(join(packageDir, 'data', `${kind}.json`), 'utf8'))
    const upstreamText = scope === 'champions'
      ? champions.text.eng[kind]
      : JSON.parse(await readFile(join(packageDir, 'data/i18n/eng', `${kind}.json`), 'utf8'))
    assert.deepEqual(payload, catalogPayload(kind, upstreamRecords, upstreamText))
  }
  assert.ok(payloads.pokemon.records.find((record) => record.id === 'charizard').learnset.includes('protect'))
  assert.ok(payloads.moves.records.some((record) => record.usable === false))
  for (const [kind, id] of [['moves', 'protect'], ['abilities', 'stench'], ['items', 'cheriberry'], ['battle-states', 'harsh-sunlight']]) {
    assert.equal(typeof payloads[kind].text[id].desc, 'string')
    assert.equal(payloads[kind].text[id].desc, champions.text.eng[kind][id].desc)
  }
  assert.equal(payloads.types.records.find((record) => record.id === 'stellar').isCanonical, false)
  assert.equal(payloads.natures.records.find((record) => record.id === 'hardy').raises, null)
  await assert.rejects(exportSnapshot(first), { code: 'EEXIST' })
})
