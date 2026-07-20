# RGB11 官方互操作与外部验收

目标一使用三层门禁，避免把“本项目内部互通”误认为 RGB 官方互操作。

## 推荐的外部钱包与工具

1. **官方 `RGB-Tools/rgb-lib`**：实际 RGB 钱包互操作端固定为 commit `538f2abaa67d7ce96be32d94092e8f1b9e3ea38e`（`0.3.0-beta.7`，RGB `0.11.1-rc.11`）。`reference/rgb-lib-wallet` 只提供薄 CLI，钱包状态、签名、Esplora 同步、Consignment 验证和余额均由上游库完成。
2. **RGB 官方 `rgb` 命令行钱包**：用于核对官方命令面和钱包派生。manifest 冻结的 `v0.11.1-alpha.3` 使用 RGB alpha.3 crates，不能代替 rc.11 parser 或 rc.11 钱包接收端。
3. **Bitlight Wallet CLI regtest 环境**：作为真实 Bitcoin carrier 端到端环境。RGB 官方集成页明确给出了 Bitlight 本地 regtest、Alice/Bob BDK wallet CLI service，以及 Esplora API `http://localhost:3002`；它提供 Bitcoin 链、签名广播和浏览器二级查证，RGB 钱包语义由上面的官方 `rgb-lib` 承担。
4. MyCitadel、BitMask、Iris 可保留人工兼容观察，但当前不作为目标一发布门禁：官方钱包页提示现有 GUI 钱包未全部更新到现代 RGB v0.11。

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

`v0.11.1-alpha.3` 的实际二进制没有在线教程示例中的旧 `rgb check` 子命令。其
`validate / accept` 只用于 alpha.3 同版本线工作流，不能作为 rc.11 文件发布门禁：
该 tag 的 `rgb-core / rgb-std / rgb-psbt` 均为 `0.11.1-alpha.3`，对未经修改的 rc.11
官方 fixture 和 Go 构造的 rc.11 fixture 都返回 `invalid file data`。rc.11 发布门禁必须
由上一节的 `0.11.1-rc.11` 官方 Rust parser 完成。

### 103 实测记录（2026-07-19）

- `bitlightlabs/bitlight-local-env` 固定在 `349260b9ec4ab9ecd5f3fb630b09349428e15a4d`，
  Alice/Bob、bitcoind、Esplora 正常运行；host 只在 loopback 暴露 `3002/5002`。
- 官方 `rgb` CLI 固定在 `a9bba35ceed7e0c4bc4e477f663ab022d7b0a23e`；Alice
  index 0 地址与 Bitlight `m/86'/1'/0'/9/0` 地址一致。
- Go 构造的 Transfer 被官方 rc.11 Rust parser 接受；consignment、contract、schema
  ID 一致，Transfer、普通 Invoice 和带 SAT20 query 的 Invoice 都 canonical round-trip。
- Alice 的 keychain 9 钱包实际签名并广播到 Bob，Esplora 确认；该项只证明 Bitcoin
  carrier 路径，不代表 RGB 资产转账。
- SDK 已将 `chaincfg.RegressionNetParams` 映射为官方 Invoice `bcrt`，Go issuance 也写入
  `ChainNet::BitcoinRegtest`。Go 生成的 regtest Contract 和 Invoice 已由冻结官方 rc.11
  Rust parser canonical round-trip。
- 官方 `rgb-lib` Alice→Bob 基线真实转账已 settled：125 单位，txid
  `c50b10c729b4539713c6df9269030bd049982d5fd335acb7b27c04600dd3f106`。
- 双向跨实现真实转账已通过：官方 Alice→Go SDK 50 单位，txid
  `693b2a13f37e2629a655c9640ee582449a2e06e1192ecb26d898fb82067a5957`；Go SDK→官方
  Bob 20 单位，txid `74e7336a9704e11c1903a8bd51ce1849e4c81b21ad92b42826ef65be53c62313`。
  两笔交易均由 Esplora 确认，后者在官方 Bob 中为 `ReceiveWitness / Settled`、收款 vout 1。
- 最终双向运行使用全新的官方 Alice/Bob、Go wallet 和 NIA 资产；官方 Bob 的 settled
  余额从 0 变为 20。双方持久化状态后，out-of-band 投递文件已删除，Go 接收方的临时
  Consignment 已压缩，但可花费状态与余额继续保留。
- 互操作过程中发现并修复 Go sender 只嵌入 `PubWitness::Txid` 的问题；witness 接收端需要
  `PubWitness::Tx` 才能在广播前把 assignment vout 与 invoice script 匹配。官方 Bob 的实际
  `provide_out_of_band_consignment` 接收是该修复的发布门禁。
- 完整远程证据位于 103 的
  `/data1/rgb11-regtest/runs/20260719-rc11-bidirectional-final/evidence/`：
  `bidirectional-summary.json`、两份 `browser-*.json`、测试日志和 `SHA256SUMS`。

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

## rc.11 钱包文件交换门禁

使用与 rc.11 wire format 匹配的官方 `rgb-lib`，按官方 Transfer 工作流完成两条方向相反的
真实文件交换；alpha.3 CLI 不得用于宣称 rc.11 文件验收通过。103 已完成前四项和双方
settled 余额核对：

1. 官方 `rgb-lib` 生成 witness Invoice，SAT20 导入 Invoice、生成 Consignment；
2. 官方 `rgb-lib` 用 `provide_out_of_band_consignment` 验证并接收该 Consignment；
3. 官方 `rgb-lib` 生成 Consignment，SAT20 Wallet SDK 导入并本地验证；
4. 对每个方向记录 Invoice、Consignment ID、contract ID、witness txid 和浏览器 snapshot SHA256；
5. 官方 `rgb-lib` 或 SAT20 Wallet SDK 使用各自钱包密钥签名并通过 Bitlight Esplora 广播；
6. 批量场景使用同一资产的两个 witness Invoice，确认一个 Consignment、两个独立 ACK，且缺少任一 ACK 都不能广播。
7. IFA `RejectListUrl` 按冻结的 `rgb-lib 0.3.0-beta.7` 执行五类互操作用例：当前 Opout、祖先 Opout、同项后置 `!allow`、允许的较新后代屏蔽旧拒绝、无关 sibling 拒绝。
8. 接收方命中 Reject List 时不投影余额，并返回钱包签名的 `Accepted=false / ReasonCode=reject-list` NACK；用户也可返回 `user-rejected`。发送方验证 NACK 后取消共享 Bitcoin 批次。

SAT20 的 `/tmp` Relay/ACK 是传输适配层，不写入 RGB Consignment 共识格式。第三方工具提供
无 transport endpoint 的 witness Invoice 时，PWA 自动进入官方 out-of-band 模式：使用文件
交换 Consignment；接收方确认接受后，由用户执行 `broadcastRGB11OutOfBand` 对应的手动确认，
不得伪造 SAT20 wallet-signed ACK。

## 首版明确边界

- RBF replacement API 延后：冻结的 `rgb-lib 0.3.0-beta.7` 未发现可直接移植的 fee-bump/replacement 实现；当前仅保留交易序列兼容性，不对外声明 RBF 功能完成。
- 批量发送已实现为官方模型可表达的“同一资产、多 witness Invoice、一个 Bitcoin transaction、一个 Consignment、逐接收者 ACK”；多资产批量和 blind batch 延后。
- 支持 Relay 模式的钱包签名 NACK：Reject List 自动拒收和用户手动拒收。官方 out-of-band 模式没有 SAT20 Relay Record，因此不伪造 DKVS NACK，由外部渠道通知拒收。
- Reject List 是 IFA 钱包策略，不改变 RGB 共识有效性。服务不可用时保持 pending 并停止发送/接收，不产生 ACK 或 NACK；生产只接受 HTTPS，regtest 仅允许 loopback HTTP。
- Transfer Consignment 不做 DKVS `/blob` 永久备份。发送方在所有接收者 settled 后删除仅用于投递的副本；拒收后删除未广播交易和双方无需保存的临时 Consignment；钱包自己的已结算 change history 和接收方可花费历史继续保留。
