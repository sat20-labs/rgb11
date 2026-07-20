use std::{
    collections::HashMap,
    env, fs,
    path::{Path, PathBuf},
    time::{SystemTime, UNIX_EPOCH},
};

use armor::AsciiArmor;
use rgb_lib::{
    AssetSchema, Assignment, BitcoinNetwork, ConsignmentExt, FileContent, RgbTransfer,
    keys::{Keys, WitnessVersion, generate_keys},
    wallet::{
        DatabaseType, Online, OnlineOptions, Recipient, RecipientInfo, RecipientType,
        RgbWalletOpsOffline, RgbWalletOpsOnline, SinglesigKeys, SyncKeychain, SyncOptions,
        SyncStrategy, Wallet, WalletData, WitnessData,
    },
};
use serde::{Deserialize, Serialize};
use serde_json::json;

type DynError = Box<dyn std::error::Error>;

#[derive(Serialize, Deserialize)]
struct WalletState {
    keys: Keys,
    funding_address: String,
}

fn state_path(data_dir: &Path) -> PathBuf {
    data_dir.join("interop-keys.json")
}

fn write_state(data_dir: &Path, state: &WalletState) -> Result<(), DynError> {
    fs::create_dir_all(data_dir)?;
    let path = state_path(data_dir);
    fs::write(&path, serde_json::to_vec_pretty(state)?)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))?;
    }
    Ok(())
}

fn read_state(data_dir: &Path) -> Result<WalletState, DynError> {
    Ok(serde_json::from_slice(&fs::read(state_path(data_dir))?)?)
}

fn load_wallet(data_dir: &Path) -> Result<(Wallet, WalletState), DynError> {
    let state = read_state(data_dir)?;
    let wallet = Wallet::load(
        data_dir.to_string_lossy().as_ref(),
        &state.keys.master_fingerprint,
        Some(state.keys.mnemonic.clone()),
    )?;
    Ok((wallet, state))
}

fn go_online(wallet: &mut Wallet, esplora_url: &str) -> Result<Online, DynError> {
    Ok(wallet.go_online(OnlineOptions {
        indexer_url: esplora_url.to_owned(),
        skip_consistency_check: false,
        vanilla_sync_lookback: 20,
    })?)
}

fn full_sync(wallet: &mut Wallet, online: Online) -> Result<(), DynError> {
    for keychain in [
        SyncKeychain::Vanilla { lookback: 20 },
        SyncKeychain::Colored,
    ] {
        wallet.sync(
            online,
            SyncOptions {
                keychain,
                strategy: SyncStrategy::FullScan,
            },
        )?;
    }
    Ok(())
}

fn arg(args: &[String], index: usize, name: &str) -> Result<String, DynError> {
    args.get(index)
        .cloned()
        .ok_or_else(|| format!("missing argument: {name}").into())
}

fn amount_arg(args: &[String], index: usize) -> Result<u64, DynError> {
    Ok(arg(args, index, "amount")?.parse()?)
}

fn expiry() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("system time after epoch")
        .as_secs()
        + 3600
}

fn main() -> Result<(), DynError> {
    let args: Vec<String> = env::args().collect();
    let command = arg(&args, 1, "command")?;
    let data_dir = PathBuf::from(arg(&args, 2, "data-dir")?);

    match command.as_str() {
        "inspect" => {
            let transfer = RgbTransfer::load_file(&data_dir)?;
            println!("{}", serde_json::to_string(&transfer)?);
        }
        "armor" => {
            let output = PathBuf::from(arg(&args, 3, "armor-output")?);
            let transfer = RgbTransfer::load_file(&data_dir)?;
            fs::write(&output, transfer.to_ascii_armored_string())?;
            println!(
                "{}",
                json!({
                    "contract_id": transfer.contract_id().to_string(),
                    "input": data_dir,
                    "output": output,
                })
            );
        }
        "dearmor" => {
            let output = PathBuf::from(arg(&args, 3, "binary-output")?);
            let armored = fs::read_to_string(&data_dir)?;
            let transfer = RgbTransfer::from_ascii_armored_str(&armored)?;
            transfer.save_file(&output)?;
            println!(
                "{}",
                json!({
                    "contract_id": transfer.contract_id().to_string(),
                    "input": data_dir,
                    "output": output,
                })
            );
        }
        "init" => {
            if state_path(&data_dir).exists() {
                let state = read_state(&data_dir)?;
                println!(
                    "{}",
                    json!({"funding_address": state.funding_address, "created": false})
                );
                return Ok(());
            }
            fs::create_dir_all(&data_dir)?;
            let keys = generate_keys(BitcoinNetwork::Regtest, WitnessVersion::Taproot);
            let mut wallet = Wallet::new(
                WalletData {
                    data_dir: data_dir.to_string_lossy().into_owned(),
                    bitcoin_network: BitcoinNetwork::Regtest,
                    database_type: DatabaseType::Sqlite,
                    max_allocations_per_utxo: 5,
                    supported_schemas: vec![AssetSchema::Nia, AssetSchema::Ifa, AssetSchema::Uda],
                },
                SinglesigKeys::from_keys(&keys, None),
            )?;
            let funding_address = wallet.get_address()?;
            write_state(
                &data_dir,
                &WalletState {
                    keys,
                    funding_address: funding_address.clone(),
                },
            )?;
            println!(
                "{}",
                json!({"funding_address": funding_address, "created": true})
            );
        }
        "address" => {
            let state = read_state(&data_dir)?;
            println!("{}", json!({"funding_address": state.funding_address}));
        }
        "sync" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            full_sync(&mut wallet, online)?;
            let balance = wallet.get_btc_balance(Some(online), true)?;
            println!("{}", serde_json::to_string(&balance)?);
        }
        "create-utxos" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            full_sync(&mut wallet, online)?;
            let created = wallet.create_utxos(online, true, Some(5), Some(10_000), 2, true)?;
            println!("{}", json!({"created": created}));
        }
        "issue-nia" => {
            let ticker = arg(&args, 3, "ticker")?;
            let name = arg(&args, 4, "name")?;
            let amount = amount_arg(&args, 5)?;
            let (wallet, _) = load_wallet(&data_dir)?;
            let asset = wallet.issue_asset_nia(ticker, name, 0, vec![amount])?;
            println!("{}", serde_json::to_string(&asset)?);
        }
        "receive-witness" => {
            let asset_id = arg(&args, 3, "asset-id or -")?;
            let amount = amount_arg(&args, 4)?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let receive = wallet.witness_receive(
                (asset_id != "-").then_some(asset_id),
                Assignment::Fungible(amount),
                expiry(),
                vec![],
                1,
            )?;
            println!("{}", serde_json::to_string(&receive)?);
        }
        "send" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let asset_id = arg(&args, 4, "asset-id")?;
            let recipient_id = arg(&args, 5, "recipient-id")?;
            let amount = amount_arg(&args, 6)?;
            let donation: bool = arg(&args, 7, "donation")?.parse()?;
            let recipient_info = RecipientInfo::new(recipient_id.clone())?;
            let witness_data =
                (recipient_info.recipient_type == RecipientType::Witness).then_some(WitnessData {
                    amount_sat: 1_000,
                    blinding: None,
                });
            let recipient_map = HashMap::from([(
                asset_id.clone(),
                vec![Recipient {
                    recipient_id,
                    witness_data,
                    assignment: Assignment::Fungible(amount),
                    transport_endpoints: vec![],
                }],
            )]);
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            full_sync(&mut wallet, online)?;
            let operation = wallet.send(online, recipient_map, donation, 2, 1, expiry())?;
            let consignment = wallet
                .get_send_consignment_path(&asset_id, &operation.txid)
                .to_string_lossy()
                .into_owned();
            println!(
                "{}",
                json!({
                    "txid": operation.txid,
                    "batch_transfer_idx": operation.batch_transfer_idx,
                    "consignment": consignment,
                    "donation": donation
                })
            );
        }
        "accept" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let consignment = arg(&args, 4, "consignment-path")?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            let refreshed = wallet.provide_out_of_band_consignment(online, consignment, vec![])?;
            println!("{}", serde_json::to_string(&refreshed)?);
        }
        "ack" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let recipient_id = arg(&args, 4, "recipient-id")?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            let result = wallet.provide_out_of_band_ack(online, recipient_id)?;
            println!("{}", serde_json::to_string(&result)?);
        }
        "refresh" => {
            let esplora = arg(&args, 3, "esplora-url")?;
            let asset_id = arg(&args, 4, "asset-id or -")?;
            let (mut wallet, _) = load_wallet(&data_dir)?;
            let online = go_online(&mut wallet, &esplora)?;
            let refreshed =
                wallet.refresh(online, (asset_id != "-").then_some(asset_id), vec![], false)?;
            println!("{}", serde_json::to_string(&refreshed)?);
        }
        "balance" => {
            let asset_id = arg(&args, 3, "asset-id")?;
            let (wallet, _) = load_wallet(&data_dir)?;
            println!(
                "{}",
                serde_json::to_string(&wallet.get_asset_balance(asset_id)?)?
            );
        }
        "assets" => {
            let (wallet, _) = load_wallet(&data_dir)?;
            println!("{}", serde_json::to_string(&wallet.list_assets(vec![])?)?);
        }
        "transfers" => {
            let asset_id = arg(&args, 3, "asset-id or -")?;
            let (wallet, _) = load_wallet(&data_dir)?;
            println!(
                "{}",
                serde_json::to_string(
                    &wallet.list_transfers((asset_id != "-").then_some(asset_id))?
                )?
            );
        }
        _ => return Err(format!("unsupported command: {command}").into()),
    }
    Ok(())
}
