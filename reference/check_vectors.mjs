import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'

const cargo = process.env.CARGO || 'cargo'
const run = spawnSync(cargo, [
  'run',
  '--manifest-path', 'reference/rust/Cargo.toml',
  '--locked',
  '--quiet',
  '--bin', 'rgb11-reference',
], {
  cwd: new URL('..', import.meta.url),
  env: process.env,
  encoding: 'utf8',
  maxBuffer: 16 * 1024 * 1024,
})

if (run.error) {
  throw new Error(`failed to start Rust oracle with ${cargo}: ${run.error.message}`, { cause: run.error })
}
if (run.status !== 0) {
  process.stderr.write(run.stderr || run.stdout || `Rust oracle exited with status ${run.status}\n`)
  process.exit(run.status ?? 1)
}

const oracle = JSON.parse(run.stdout)
const fixture = JSON.parse(readFileSync(new URL('../testvectors/rc11/core.json', import.meta.url), 'utf8'))
const canonical = JSON.stringify(oracle)
const digest = createHash('sha256').update(canonical).digest('hex')

if (Object.keys(oracle).length !== fixture.oracle_vector_count) {
  throw new Error(`Rust oracle vector count ${Object.keys(oracle).length}, expected ${fixture.oracle_vector_count}`)
}
if (digest !== fixture.oracle_canonical_sha256) {
  throw new Error(`Rust oracle digest ${digest}, expected ${fixture.oracle_canonical_sha256}`)
}
for (const [key, expected] of Object.entries(fixture.vectors)) {
  if (oracle[key] !== expected) {
    throw new Error(`Rust oracle mismatch for ${key}`)
  }
}

console.log(JSON.stringify({
  status: 'ok',
  oracleVectors: Object.keys(oracle).length,
  goDifferentialVectors: Object.keys(fixture.vectors).length,
  oracleCanonicalSHA256: digest,
}, null, 2))
