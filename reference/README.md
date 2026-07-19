# Rust reference oracle

该目录只用于开发期 differential tests。它锁定官方 RGB 0.11.1
crate，生成 Go 测试所使用的字节级向量；生产运行时不依赖 Rust。

运行：

```bash
CARGO_HOME=/private/tmp/rgb11-cargo \
RUSTUP_HOME=/private/tmp/rgb11-rustup \
/private/tmp/rgb11-cargo/bin/cargo run \
  --manifest-path reference/rust/Cargo.toml --locked \
  --bin rgb11-reference
```

运行冻结 oracle 门禁（校验全部 75 个 Rust 输出的 canonical digest，并逐项
比较 Go 测试直接使用的 45 个差分向量）：

```bash
CARGO=/private/tmp/rgb11-cargo/bin/cargo \
CARGO_HOME=/private/tmp/rgb11-cargo \
RUSTUP_HOME=/private/tmp/rgb11-rustup \
CARGO_NET_OFFLINE=true \
node reference/check_vectors.mjs
```
