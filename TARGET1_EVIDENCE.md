# RGB11 目标一开发验收证据

基线：`rgb-lib 0.3.0-beta.7`，RGB `0.11.1-rc.11` release set。目标范围仅为
PWA / Wallet SDK 原生管理 Bitcoin L1 RGB11；STP 和 SatoshiNet RGB11 入口保持关闭。

## 已确认的产品约束

- 官方 asset ID 写入 SAT20 `AssetName.Ticker` 时只删除固定 `rgb:` 前缀，余下字符原样保存，不再编码。
- wallet head 的权威来自钱包自己的 DKVS `/personal` record 签名，以及 DKVS 对该 key 选出的最新有效 seq。
- head payload 只有 `version / wallet_id / seq / state_hash / operation_id`；不使用 `/system` checkpoint key，也不重复内嵌签名。
- 独立 Go module 为 `github.com/sat20-labs/rgb11`。
- Tapret carrier 使用当前普通 P2TR/BIP86 子账户的 internal key，并与对应普通收款/找零地址使用同一个 derivation index；新建转移优先 Opret。
- 首版发行只开放官方 NIA、IFA、UDA。CFA 保留导入和验证能力，但不开放发行 API/UI。

## 验收矩阵

| 验收项 | 实现证据 | 自动化证据 |
|---|---|---|
| 冻结官方实现 | `UPSTREAM_MANIFEST.json` 固定 repo/tag/commit/crate checksum | Rust oracle 使用 `Cargo.lock --locked` |
| 官方 Wallet schema | NIA、IFA、CFA、UDA contract import/validation；NIA、IFA、UDA issuance | 四份官方 contract fixture；四 schema genesis 与 confidential transition 差分；CFA issuance rejection |
| Go/Rust 一致性 | Strict Encoding、ID、Seal、Anchor、Invoice、PSBT key、Contract、Transition、Bundle | 75 个 Rust oracle 输出，45 个 Go 直接向量，canonical SHA256 门禁 |
| L1 发行 | Wallet SDK 选择本钱包已确认 plain Bitcoin UTXO，构造并本地验证 contract | NIA、IFA、UDA 发行、余额投影、`reason=rgb` lock 测试；SDK/PWA 拒绝 CFA 发行 |
| L1 收发 | Invoice、Opret witness、PSBT、Consignment、Relay、ACK gate、广播 | 官方 NIA Relay/ACK E2E；NIA/IFA/CFA/UDA 发行后 send/receive E2E |
| Tapret / Opret | Go Engine 同时验证两种 carrier binding；新转账优先 Opret；Tapret internal key、普通地址与 carrier 绑定同一 BIP86 derivation index | anchor、跨 index 拒绝、同 index Tapret key-path signing、Opret witness tests |
| 统一钱包模型 | RGB11 复用 `AssetName/AssetInfo/Decimal/TxOutput/TickerInfo/UtxoLocker`，余额从 outputs 重建 | projection/store/adapter tests；不存在 Indexer RGB balance API |
| 防误花 | carrier 用 `reason=rgb`，pending fee input 用 `pending-rgb` | 普通 fee selection 排除 RGB carrier；恢复后重建 lock |
| Bitcoin evidence | L1 Indexer 只提供 UTXO、raw tx、tx status、outspend、tip/header、fee、broadcast facts | HTTP contract tests；Wallet evidence adapter tests |
| 生命周期 | transfer 状态覆盖 prepared/relayed/accepted/broadcast/pending/settled/conflicted；未知 spend fail closed | ACK-before-broadcast、reorg rollback、conflict/inconsistent tests |
| 两设备恢复 | 加密 immutable snapshot + 钱包签名 latest head；stale writer 外部副作用 guard | 两设备收敛、旧 seq 冲突、恢复 allocation/balance/lock tests |
| PWA | 统一 L1 列表、NIA/IFA/UDA 发行、导入、Invoice、收发、备份/恢复、proof 详情、transfer monitor | WASM export smoke、CFA 发行入口缺失门禁、`vue-tsc`、production build |
| STP 隔离 | RGB11 不进入普通 deposit/splicing/聪网操作 | PWA RGB11 分支只暴露 L1 send/receive；SDK STP preservation guard tests |

## 回归命令

```bash
cd /Users/yingfeng/github/rgb11
go test ./... -count=1
node reference/check_vectors.mjs

cd /Users/yingfeng/github/sat20wallet/sdk
go test ./wallet ./wallet/rgb11 -count=1
go test ./... -run '^$' -count=1

cd /Users/yingfeng/github/sat20wallet/pwa
npm run verify:rgb11-l1
npm run build

cd /Users/yingfeng/github/indexer
go test ./rpcserver/bitcoind ./share/bitcoin_rpc -count=1
```

第三方钱包互操作的代码边界是标准 RGB Armor、Invoice、Consignment、ACK 和
PSBT proprietary fields，不依赖 SAT20 私有数据库格式。发布前仍应使用计划支持的
第三方钱包版本执行一次真实文件交换验收；该外部产品组合不属于本仓库自动化测试环境。

SDK 的全量 `go test ./...` 还会启动既有 SatoshiNet E2E。当前工作区该套件在非 RGB11
ascending anchor 阶段报 `binding sats 1 does not match anchor 0`；RGB11 wallet tests
以及全包只编译门禁独立通过，不能把该既有 E2E 失败记作 RGB11 验收通过。
