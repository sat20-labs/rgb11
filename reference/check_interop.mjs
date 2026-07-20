import { spawnSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const root = new URL('..', import.meta.url)
const cargo = process.env.CARGO || 'cargo'
const go = process.env.GO || 'go'
const temp = mkdtempSync(join(tmpdir(), 'rgb11-interop-'))
const transferPath = join(temp, 'go-built-transfer.rgba')
const regtestContractPath = join(temp, 'go-built-regtest-contract.rgba')

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
    process.exit(result.status ?? 1)
  }
  return result.stdout.trim()
}

try {
  const goResult = JSON.parse(run(go, [
	'run', './cmd/rgb11-interop-fixture',
	'--output', transferPath,
	'--regtest-output', regtestContractPath,
  ], { ...process.env, GOCACHE: process.env.GOCACHE || join(temp, 'go-build') }))
  const rustBase = [
    'run', '--manifest-path', 'reference/rust/Cargo.toml', '--locked', '--quiet', '--bin', 'rgb11-reference', '--',
  ]
  const rustTransfer = JSON.parse(run(cargo, [...rustBase, 'inspect-transfer', transferPath]))
	const rustRegtestContract = JSON.parse(run(cargo, [...rustBase, 'inspect-contract', regtestContractPath]))
  const rustInvoice = JSON.parse(run(cargo, [...rustBase, 'inspect-invoice', goResult.invoice]))
  const rustSAT20Invoice = JSON.parse(run(cargo, [...rustBase, 'inspect-invoice', goResult.sat20_invoice]))
	const rustRegtestInvoice = JSON.parse(run(cargo, [...rustBase, 'inspect-invoice', goResult.regtest_invoice]))

  for (const field of ['id', 'contract_id', 'schema_id']) {
    if (rustTransfer[field] !== goResult[field]) {
      throw new Error(`Go -> official Rust transfer mismatch for ${field}: ${goResult[field]} != ${rustTransfer[field]}`)
    }
  }
  if (!rustTransfer.canonical_roundtrip || rustInvoice.invoice !== goResult.invoice ||
      rustSAT20Invoice.invoice !== goResult.sat20_invoice ||
	  !rustRegtestContract.canonical_roundtrip || rustRegtestContract.chain_net !== 'bcrt' ||
	  rustRegtestContract.contract_id !== goResult.regtest_contract_id ||
	  rustRegtestContract.schema_id !== goResult.regtest_schema_id ||
	  rustRegtestInvoice.invoice !== goResult.regtest_invoice) {
    throw new Error('official Rust parser did not canonically round-trip Go artifacts')
  }
  console.log(JSON.stringify({
    status: 'ok',
    walletOracle: 'RGB-Tools/rgb-lib 0.3.0-beta.7 dependency set',
    transfer: rustTransfer,
    invoice: rustInvoice.invoice,
    sat20Invoice: rustSAT20Invoice.invoice,
	regtestContract: rustRegtestContract,
	regtestInvoice: rustRegtestInvoice.invoice,
    goPayloadSHA256: goResult.payload_sha256,
  }, null, 2))
} finally {
  rmSync(temp, { recursive: true, force: true })
}
