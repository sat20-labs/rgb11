# Official rgb-lib regtest wallet harness

This small CLI pins the official [`RGB-Tools/rgb-lib`](https://github.com/RGB-Tools/rgb-lib)
source at commit `538f2abaa67d7ce96be32d94092e8f1b9e3ea38e`
(`0.3.0-beta.7`, RGB `0.11.1-rc.11`). It is only an interoperability
harness: asset issuance, wallet state, transaction construction, signing,
Esplora synchronization and strict-consignment validation are all performed by
the upstream library.

The harness intentionally uses out-of-band exchange (`transport_endpoints=[]`).
Consignments may therefore be copied to the counterparty and deleted after the
receiver has validated and persisted the transfer.

`armor <binary> <output>` and `dearmor <armor> <output>` convert only the
official file envelope. Both directions are parsed and serialized by the
upstream `Transfer` type; they do not translate or reinterpret RGB state.

Each wallet data directory contains `interop-keys.json` with test-only private
key material. The file is created with mode `0600`; never reuse these wallets
outside an isolated regtest.

Run `cargo run --release -- <command> ...`. The available commands are visible
in `src/main.rs`; the automated 103 runner invokes them with the local Esplora
URL.

The Wallet SDK live gate is build-tagged so ordinary unit tests stay offline:

```bash
RGB11_REGTEST_ESPLORA=http://127.0.0.1:3002 \
RGB11_REGTEST_OFFICIAL_BIN=/path/to/rgb-lib-wallet-interop \
RGB11_REGTEST_OFFICIAL_ALICE=/path/to/alice \
RGB11_REGTEST_OFFICIAL_BOB=/path/to/bob \
RGB11_REGTEST_COMPOSE_DIR=/path/to/bitlight-local-env \
RGB11_REGTEST_ASSET_ID='rgb:...' \
RGB11_REGTEST_EVIDENCE_DIR=/path/to/evidence \
SAT20WALLET_RUN_LIVE_NETWORK_TESTS=1 \
go test -tags rgb11regtest ./wallet \
  -run '^TestRGB11RegtestOfficialBidirectional$' -count=1 -v
```

The test executes official Alice → Go SDK → official Bob, checks both Bitcoin
confirmations, the exact assignment and balances, and requires Bob's receive to
be `Settled`. It closes and reopens the Go database and recreates the Go wallet
from its test mnemonic; each official CLI command also reloads its wallet from
disk. Both balances and the Go allocation proof identities must survive restart.

Run the same gate separately for `RGB11_REGTEST_SCHEMA=NIA`, `IFA`, and `UDA`,
using fresh wallets and a freshly issued upstream asset for each run. Fungible
cases transfer 50 units to Go and 20 to Bob; UDA transfers the unique token in
both directions. `issue-ifa <dir> <ticker> <name> <amount> <inflation-right>` and
`issue-uda <dir> <ticker> <name>` invoke the pinned upstream issuance APIs.
`send` accepts a final `fungible` or `uda` assignment argument. An unknown UDA
receiver uses `receive-witness <dir> - 1 any`, the upstream `Assignment::Any`
workflow; acceptance must subsequently resolve it to `NonFungible`.

For a native local regtest, replace the compose variable with:

```bash
RGB11_REGTEST_BITCOIN_CLI=/path/to/bitcoin-cli \
RGB11_REGTEST_BITCOIN_DATADIR=/path/to/disposable-chain \
RGB11_REGTEST_BITCOIN_RPC_PORT=18453 \
SAT20WALLET_RUN_LIVE_NETWORK_TESTS=1 \
RGB11_REGTEST_SCHEMA=NIA \
go test -tags rgb11regtest ./wallet \
  -run '^TestRGB11RegtestOfficialBidirectional$' -count=1 -v
```

Also set the Esplora, official binary, wallet, asset and evidence variables from
the first example.

All services must bind to loopback and the chain must be Bitcoin `regtest`.
The local runtime used for this extension pins Bitcoin Core `28.1` (verified
release archive SHA256) and Blockstream/electrs commit
`dd09701e84dc88a2c58a1365f798222b66e13772` with its locked dependencies. This
changes neither pinned RGB oracle version. A missing live environment is not a
passing interoperability result: when the live flag is explicitly enabled,
missing required configuration fails the test.

Fund the official wallets with ordinary confirmed outputs and mine preparation
blocks to the bootstrap wallet. A newly mined coinbase assigned to an official
wallet can be selected before maturity and rejected by Bitcoin Core. For a fresh
regtest without fee estimates, set an explicit funding RPC `fee_rate` (the local
run used 2 sat/vB).

The SDK gate refreshes the confirmed receive state before spending, loads both
the main address and actual witness address from Esplora, and selects assignment
type 4000 by its recipient script. IFA control allocations are tracked separately.
The Esplora adapter checks transaction existence via `/tx/<txid>`: an unknown
txid's `/status` response of `confirmed=false` does not prove mempool visibility.
`TestRegtestEsploraTransactionPresence` locks this distinction with a reproduced
negative case and a known unconfirmed transaction.
