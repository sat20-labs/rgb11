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
go test -tags rgb11regtest ./wallet \
  -run '^TestRGB11RegtestOfficialBidirectional$' -count=1 -v
```

The test mines only on the disposable Bitlight regtest, executes official
Alice → Go SDK → official Bob, checks both Bitcoin confirmations, and requires
the official Bob transfer to become `Settled`.
