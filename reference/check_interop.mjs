import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdtempSync, rmSync, readFileSync, mkdirSync, writeFileSync, cpSync, existsSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, join } from 'node:path'

const root = new URL('..', import.meta.url)
const cargo = process.env.CARGO || 'cargo'
const go = process.env.GO || 'go'
const temp = mkdtempSync(join(tmpdir(), 'rgb11-interop-'))
const transferPath = join(temp, 'go-built-transfer.rgba')
const regtestContractPath = join(temp, 'go-built-regtest-contract.rgba')
const validationDir = join(temp, 'validation')
const differential = []

const run = (command, args, env = process.env) => {
  const result = spawnSync(command, args, {
    cwd: root,
    env,
    encoding: 'utf8',
    maxBuffer: 32 * 1024 * 1024,
  })
  if (result.error) throw result.error
  if (result.status !== 0) {
    process.stderr.write(result.stderr || result.stdout)
    throw new Error(`${command} exited with status ${result.status ?? 1}`)
  }
  return result.stdout.trim()
}

try {
  const goResult = JSON.parse(run(go, [
	'run', './cmd/rgb11-interop-fixture',
	'--output', transferPath,
	'--regtest-output', regtestContractPath,
	'--validation-dir', validationDir,
  ], { ...process.env, GOCACHE: process.env.GOCACHE || join(temp, 'go-build') }))
  const rustBase = [
    'run', '--manifest-path', 'reference/rust/Cargo.toml', '--locked', '--quiet', '--bin', 'rgb11-reference', '--',
  ]
  const cases = JSON.parse(readFileSync(join(validationDir, 'cases.json'), 'utf8'))
  const original = cases.find(item => item.name === 'nia-official-transfer')
  if (!original) throw new Error('original official Transfer validation case is missing')
  const rustTransfer = JSON.parse(run(cargo, [...rustBase, 'inspect-transfer', transferPath, original.evidence]))
  if (rustTransfer.outcome !== 'valid') throw new Error(`official full validation rejected Go Transfer: ${rustTransfer.error}`)
  for (const item of cases) {
    const official = JSON.parse(run(cargo, [...rustBase, item.binary ? 'inspect-transfer-payload' : 'inspect-transfer', item.path, item.evidence]))
    const { payload_hex, ...result } = official
    differential.push({ name: item.name, expected: item.expected, goOutcome: item.go_outcome, goStage: item.go_stage, goError: item.go_error, official: result })
    if (official.outcome !== item.go_outcome || official.outcome !== item.expected) {
      const { payload_hex, ...failure } = official
      throw new Error(`${item.name}: official=${official.outcome}, Go=${item.go_outcome}, expected=${item.expected}; ${JSON.stringify(failure)}`)
    }
    if (item.payload_hex && official.payload_hex !== item.payload_hex) {
      throw new Error(`${item.name}: Go and official strict payload bytes differ`)
    }
    if (item.go_stage === 'schema_script' && !official.error?.includes('ScriptFailure(')) {
      throw new Error(`${item.name}: official validation did not reach the schema script: ${official.error}`)
    }
    process.stderr.write(`validated ${item.name}: ${official.outcome}\n`)
  }
  if (createHash('sha256').update(Buffer.from(rustTransfer.payload_hex, 'hex')).digest('hex') !== goResult.payload_sha256) {
    throw new Error('Go and official Transfer payload bytes differ')
  }
  const rustRegtestContract = JSON.parse(run(cargo, [...rustBase, 'inspect-contract', regtestContractPath]))
  const rustInvoice = JSON.parse(run(cargo, [...rustBase, 'inspect-invoice', goResult.invoice]))
	const rustRegtestInvoice = JSON.parse(run(cargo, [...rustBase, 'inspect-invoice', goResult.regtest_invoice]))

  for (const field of ['id', 'contract_id', 'schema_id']) {
    if (rustTransfer[field] !== goResult[field]) {
      throw new Error(`Go -> official Rust transfer mismatch for ${field}: ${goResult[field]} != ${rustTransfer[field]}`)
    }
  }
  if (!rustTransfer.canonical_roundtrip || rustInvoice.invoice !== goResult.invoice ||
	  !rustRegtestContract.canonical_roundtrip || !rustRegtestContract.consensus_valid ||
	  rustRegtestContract.chain_net !== 'bcrt' ||
	  rustRegtestContract.contract_id !== goResult.regtest_contract_id ||
	  rustRegtestContract.schema_id !== goResult.regtest_schema_id ||
	  rustRegtestInvoice.invoice !== goResult.regtest_invoice) {
    throw new Error('official Rust parser did not canonically round-trip Go artifacts')
  }
  console.log(JSON.stringify({
    status: 'ok',
    protocolOracle: 'rgb-protocol 0.11.1 stable release set',
    transfer: { ...rustTransfer, payload_hex: undefined },
    invoice: rustInvoice.invoice,
	regtestContract: rustRegtestContract,
	regtestInvoice: rustRegtestInvoice.invoice,
    goPayloadSHA256: goResult.payload_sha256,
    fullValidationCases: differential,
  }, null, 2))
} finally {
  if (process.env.RGB11_INTEROP_EVIDENCE_DIR) {
    const savedInputs = join(process.env.RGB11_INTEROP_EVIDENCE_DIR, 'inputs')
    mkdirSync(savedInputs, { recursive: true })
    for (const name of ['go-built-transfer.rgba', 'go-built-regtest-contract.rgba', 'validation']) {
      if (existsSync(join(temp, name))) cpSync(join(temp, name), join(savedInputs, name), { recursive: true })
    }
    const savedManifest = join(savedInputs, 'validation', 'cases.json')
    if (existsSync(savedManifest)) {
      const savedCases = JSON.parse(readFileSync(savedManifest, 'utf8'))
      for (const item of savedCases) {
        item.path = join(savedInputs, 'validation', basename(item.path))
        item.evidence = join(savedInputs, 'validation', basename(item.evidence))
      }
      writeFileSync(savedManifest, JSON.stringify(savedCases, null, 2) + '\n')
    }
    writeFileSync(join(process.env.RGB11_INTEROP_EVIDENCE_DIR, 'differential-summary.json'), JSON.stringify(differential, null, 2) + '\n')
  }
  rmSync(temp, { recursive: true, force: true })
}
