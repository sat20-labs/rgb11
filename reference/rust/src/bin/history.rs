// Offline history oracle against the exact 0.11.1 dependencies in Cargo.toml.
// The input is an SDK-generated test consignment, never a live wallet.
use armor::AsciiArmor;
use bitcoin::{Transaction, Txid};
use rgbstd::containers::{ConsignmentExt, Transfer};
use rgbstd::validation::{ResolveWitness, ValidationConfig, WitnessResolverError, WitnessStatus};
use rgbstd::vm::{WitnessOrd, WitnessPos};
use rgbstd::ChainNet;
use std::{collections::BTreeMap, env, fs, num::NonZeroU32, path::Path};

struct Resolver {
    txs: BTreeMap<Txid, Transaction>,
    mode: String,
}
impl ResolveWitness for Resolver {
    fn resolve_witness(&self, id: Txid) -> Result<WitnessStatus, WitnessResolverError> {
        if self.mode == "unresolved" {
            return Ok(WitnessStatus::Unresolved);
        }
        let tx = self.txs.get(&id).expect("fixture witness").clone();
        let ord = if self.mode == "archived" {
            WitnessOrd::Archived
        } else {
            WitnessOrd::Mined(
                WitnessPos::bitcoin(NonZeroU32::new(100).unwrap(), 1700000000).unwrap(),
            )
        };
        Ok(WitnessStatus::Resolved(tx, ord))
    }
    fn check_chain_net(&self, _: ChainNet) -> Result<(), WitnessResolverError> {
        Ok(())
    }
}
fn main() {
    let args: Vec<_> = env::args().collect();
    let path = Path::new(&args[1]);
    let transfer = Transfer::from_ascii_armored_str(&fs::read_to_string(path).unwrap()).unwrap();
    let mode = args.get(2).cloned().unwrap_or_else(|| "mined".into());
    let txs = transfer
        .bundled_witnesses()
        .map(|b| {
            let id = b.pub_witness.txid();
            let raw = fs::read(path.parent().unwrap().join(format!("{id}.tx"))).unwrap();
            let tx: Transaction = bitcoin::consensus::deserialize(&raw).unwrap();
            assert_eq!(tx.compute_txid(), id);
            (id, tx)
        })
        .collect();
    let config = ValidationConfig {
        chain_net: transfer.genesis().chain_net,
        trusted_typesystem: transfer.types.clone(),
        ..Default::default()
    };
    let result = transfer.validate(
        &Resolver {
            txs,
            mode: mode.clone(),
        },
        &config,
    );
    let valid = result.is_ok();
    println!(
        "{}",
        serde_json::json!({"file": path.file_name().unwrap().to_str(), "mode": mode, "valid": valid, "error": result.err().map(|e| format!("{e:?}"))})
    );
    assert_eq!(valid, mode == "mined");
}
