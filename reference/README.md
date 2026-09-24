# Rust reference oracle

该目录只用于开发期 differential tests。它锁定官方 RGB 0.11.1
正式版 crate，生成 Go 测试所使用的字节级向量；生产运行时不依赖 Rust。
`testvectors/rc11` 保留原始 rc.11 证据；正式版 oracle 重新生成的向量摘要与之相同。

运行：

```bash
CARGO_HOME=/private/tmp/rgb11-cargo-0111 cargo run \
  --manifest-path reference/rust/Cargo.toml --locked \
  --bin rgb11-reference
```

运行冻结 oracle 门禁（校验全部 75 个 Rust 输出的 canonical digest，并逐项
比较 Go 测试直接使用的 45 个差分向量）：

```bash
CARGO_HOME=/private/tmp/rgb11-cargo-0111 \
node reference/check_vectors.mjs
```
