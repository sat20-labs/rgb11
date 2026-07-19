# SAT20 RGB11 Go Engine

`github.com/sat20-labs/rgb11` 是 SAT20 Wallet SDK 使用的 RGB 0.11.1 Go 实现。

当前冻结基线：

- `RGB-Tools/rgb-lib` `0.3.0-beta.7`；
- RGB consensus / ops / schemas / API `0.11.1-rc.11`；
- RGB strict encoding / strict types `1.0.2`。

实现原则：

- 共识结构、严格编码、ID、Seal、Anchor、Consignment 和 PSBT 字段以冻结的官方 Rust 实现为准；
- Go 运行时不依赖 Rust、SQLite、Wallet SDK 或 Indexer；
- Wallet SDK、Indexer、DKVS 和 PWA 通过 adapter 接入；
- `rgb:` asset ID 进入 SAT20 `AssetName.Ticker` 时只去掉固定前缀，不做额外编码；
- wallet head 只保存恢复状态所需的 wallet、seq、state hash 和 snapshot/operation id，由钱包自己的 DKVS `/personal` 记录签名并按有效序号选择最新记录，不引入 predecessor hash 或 `/system` checkpoint key；
- 未通过 Rust differential suite 的代码不得标记为 production-ready。

当前载体边界：

- 接收、导入和验证路径支持 Opret 与 Tapret commitment；
- Wallet SDK 可用同一普通 P2TR 子账户的 BIP86 internal key 签名并消费 Tapret carrier；
- 新建转移当前遵循冻结版本的 Opret 默认 commitment method，不另建 RGB 专用派生路径。

运行基础测试：

```bash
go test ./...
```

上游源版本、crate checksum 和翻译状态记录在 `UPSTREAM_MANIFEST.json`。
