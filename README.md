# SAT20 RGB11 Go Engine

`github.com/sat20-labs/rgb11` 是 SAT20 Wallet SDK 使用的 RGB 0.11.1 Go 实现。

> 本仓库是对冻结 RGB Rust 基线的独立 Go 重实现，不是 RGB 上游官方 Go SDK，也不是 Rust FFI wrapper。

## 协议版本

- SAT20 协议名：`rgb11`；
- 协议目标：RGB `0.11.1`；
- 共识、operations、invoicing、schema、PSBT/API 冻结基线：`0.11.1`；
- 钱包互操作 oracle：`RGB-Tools/rgb-lib 0.3.0-beta.7`；
- Strict Encoding / Strict Types：`1.0.4`；
- RGB `0.12` 不属于 `rgb11` 的兼容升级，未来必须使用独立 `rgb12` 协议空间。

## 冻结的上游 Rust 源码

| 领域 | 版本 | 精确源码 |
| --- | --- | --- |
| Consensus、operation ID、seal、commitment | `rgb-consensus 0.11.1` | [`rgb-protocol/rgb-consensus@32a67862`](https://github.com/rgb-protocol/rgb-consensus/commit/32a67862aef0f4c7a1fdc7834a3555d702f1bdf0) |
| Operations、consignment、invoicing | `rgb-ops` / `rgb-invoicing 0.11.1` | [`rgb-protocol/rgb-ops@8bcdbf2f`](https://github.com/rgb-protocol/rgb-ops/commit/8bcdbf2fd706782948a4cbd799639709a4ed10d9) |
| PSBT utilities / API | `rgb-psbt-utils 0.11.1` | [`rgb-protocol/rgb-api@102f4b09`](https://github.com/rgb-protocol/rgb-api/commit/102f4b09efa8f0e5987e60714364ba693c89def7) |
| NIA、IFA、CFA、UDA schema | `rgb-schemas 0.11.1` | [`rgb-protocol/rgb-schemas@906817a0`](https://github.com/rgb-protocol/rgb-schemas/commit/906817a0c24a6bb7d2e53ceac9e2c185ec70a5ff) |
| Strict Encoding | `1.0.4` | [`rgb-protocol/rgb-strict-encoding@aa90bf35`](https://github.com/rgb-protocol/rgb-strict-encoding/commit/aa90bf353f53e8220aaacec4a671545808da05ae) |
| Strict Types | `1.0.4` | [`rgb-protocol/rgb-strict-types@9eb3b484`](https://github.com/rgb-protocol/rgb-strict-types/commit/9eb3b484f23b01c70f2a940bda07a207377b3864) |

钱包互操作主要参考：

- [`RGB-Tools/rgb-lib 0.3.0-beta.7@538f2aba`](https://github.com/RGB-Tools/rgb-lib/commit/538f2abaa67d7ce96be32d94092e8f1b9e3ea38e)；
- [`RGB-WG/rgb v0.11.1-alpha.3@a9bba35c`](https://github.com/RGB-WG/rgb/commit/a9bba35ceed7e0c4bc4e477f663ab022d7b0a23e)，仅用于 CLI 命令面和派生路径人工核对，不能替代正式版 parser。

`rgb-lib 0.3.0-beta.7` 仍依赖 `rc.11` crates，保留为历史钱包互操作证据；正式版协议门禁使用本仓库锁定的 `0.11.1` Rust oracle。`rc.11` 备份可由正式版引擎读取，新写入使用正式版 build ID。

精确版本、commit、crate SHA256、翻译映射和主动差异记录在 [`UPSTREAM_MANIFEST.json`](./UPSTREAM_MANIFEST.json)。官方互操作门禁和实测证据见 [`OFFICIAL_INTEROP.md`](./OFFICIAL_INTEROP.md)。

## 实现原则

- 共识结构、严格编码、ID、Seal、Anchor、Consignment 和 PSBT 字段以冻结的官方 Rust 实现为准；
- Go 运行时不依赖 Rust、SQLite、Wallet SDK 或 Indexer；
- Wallet SDK、Indexer、DKVS 和 PWA 通过 adapter 接入；
- `rgb:` asset ID 进入 SAT20 `AssetName.Ticker` 时只去掉固定前缀，不做额外编码；
- wallet head 只保存恢复状态所需的 wallet、seq、state hash 和 snapshot/operation id，由钱包自己的 DKVS `/personal` 记录签名并按有效序号选择最新记录，不引入 predecessor hash 或 `/system` checkpoint key；
- 未通过 Rust differential suite、官方 parser round-trip 和既有钱包互操作门禁的代码不得标记为 production-ready。

## 当前载体边界

- 接收、导入和验证路径支持 Opret 与 Tapret commitment；
- Tapret carrier 直接使用当前普通 P2TR/BIP86 子账户的 internal key；carrier binding 保存该普通收款/找零地址所用的同一个 derivation index，并按该 index 验证和签名；
- 新建转移优先使用 Opret，不另建 RGB 专用派生路径。

## 首版发行边界

- 发行入口只开放官方 NIA、IFA、UDA；
- CFA 仍可按官方 schema 导入和验证，但首版 SDK/PWA 不提供 CFA 发行入口。

运行基础测试：

```bash
go test ./...
```
