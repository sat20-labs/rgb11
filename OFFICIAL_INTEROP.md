# RGB11 官方互操作与外部验收

目标一使用三层门禁，避免把“本项目内部互通”误认为 RGB 官方互操作。

## 推荐的外部钱包与工具

1. **RGB 官方 `rgb` 命令行钱包**：作为首选的独立实现和文件交换对端。它最适合自动化检查 Invoice、Consignment 的解析、canonical round-trip，以及冻结版本实际提供的 `invoice / transfer / validate / accept / finalize` 工作流。
2. **Bitlight Wallet CLI regtest 环境**：作为真实钱包端到端对端。RGB 官方集成页明确给出了 Bitlight 本地 regtest、Alice/Bob wallet CLI service，以及 Esplora API `http://localhost:3002`；适合验证两钱包收发与浏览器二级查证。
3. MyCitadel、BitMask、Iris 可保留人工兼容观察，但当前不作为目标一发布门禁：官方钱包页提示现有 GUI 钱包未全部更新到现代 RGB v0.11，并优先推荐命令行工具。

## 自动门禁

### 1. 冻结版本的 Rust/Go 差分

```bash
cd /Users/yingfeng/github/rgb11
node reference/check_vectors.mjs

CARGO=/tmp/rgb11-cargo-home/bin/cargo \
CARGO_HOME=/tmp/rgb11-cargo-home \
RUSTUP_HOME=/tmp/rgb11-rustup-home \
GOCACHE=/tmp/rgb11-go-build \
node reference/check_interop.mjs
```

第二条命令由 Go 构造真实 Transfer Consignment，再交给冻结的官方 Rust
`0.11.1-rc.11` parser，比较 consignment、contract、schema ID 和 Invoice canonical
round-trip。它不是对 Go 结果的自解析。

冻结的官方 CLI 也必须从 manifest 指定的 commit 构建并核对命令面：

```bash
cd /path/to/RGB-WG/rgb
CARGO_HOME=/tmp/rgb11-cargo-home \
RUSTUP_HOME=/tmp/rgb11-rustup-home \
cargo build --locked --release -p rgb-wallet

cd /Users/yingfeng/github/rgb11
node reference/check_official_cli.mjs \
  /path/to/RGB-WG/rgb/target/release/rgb
```

`v0.11.1-alpha.3` 的实际二进制没有在线教程示例中的旧 `rgb check` 子命令；本验收
固定使用该版本存在的 `validate / accept`，不能用一个不存在的命令作为发布门禁。

### 2. 浏览器二级验证

先启动 RGB 官方集成页认可的 Bitlight regtest 环境，然后执行：

```bash
cd /Users/yingfeng/github/rgb11
go run ./cmd/rgb11-browser-check \
  -manifest ./UPSTREAM_MANIFEST.json \
  -endpoint bitlight-regtest-esplora \
  -txid <witness_txid> \
  -expected-hex <expected_raw_tx_hex> \
  -output ./browser-snapshot.json
```

二级查证只比较 tx status、raw transaction、outspends 和响应 SHA256；它永远不成为
RGB 资产余额或共识权威。远程 endpoint 必须使用 HTTPS；manifest 中仅允许 loopback
HTTP 的本地 Bitlight regtest。

## 发布前人工文件交换

按官方 Transfer 工作流完成两条方向相反的真实文件交换：

1. 官方 `rgb` / Bitlight CLI 生成 witness Invoice，SAT20 导入 Invoice、生成 Consignment；
2. 官方 CLI 先用 `validate` 验证，再用 `accept` 接收该 Consignment；
3. 官方 CLI 生成 Consignment，SAT20 PWA 导入并本地验证；
4. 对每个方向记录 Invoice、Consignment ID、contract ID、witness txid 和浏览器 snapshot SHA256；
5. 官方 CLI 用 `finalize --publish`（或配套 Bitcoin wallet）发布已签名 PSBT；
6. 批量场景使用同一资产的两个 witness Invoice，确认一个 Consignment、两个独立 ACK，且缺少任一 ACK 都不能广播。

SAT20 的 `/tmp` Relay/ACK 是传输适配层，不写入 RGB Consignment 共识格式。第三方工具提供
无 transport endpoint 的 witness Invoice 时，PWA 自动进入官方 out-of-band 模式：使用文件
交换 Consignment；接收方确认接受后，由用户执行 `broadcastRGB11OutOfBand` 对应的手动确认，
不得伪造 SAT20 wallet-signed ACK。

## 首版明确边界

- RBF replacement API 延后：冻结的 `rgb-lib 0.3.0-beta.7` 未发现可直接移植的 fee-bump/replacement 实现；当前仅保留交易序列兼容性，不对外声明 RBF 功能完成。
- 批量发送已实现为官方模型可表达的“同一资产、多 witness Invoice、一个 Bitcoin transaction、一个 Consignment、逐接收者 ACK”；多资产批量和 blind batch 延后。
- 不提供 NACK/拒收业务流程；接收方只有完整验证成功后才生成 ACK。
- Transfer Consignment 不做 DKVS `/blob` 永久备份。发送方在所有接收者 settled 后删除仅用于投递的副本；钱包自己的 change history 和接收方可花费历史继续保留。
