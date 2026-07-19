use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::str::FromStr;

use armor::AsciiArmor;
use bitcoin::consensus::serialize;
use bitcoin::key::UntweakedPublicKey;
use bitcoin::psbt::raw::ProprietaryKey;
use bitcoin::{absolute, transaction, Amount as BtcAmount, OutPoint, Psbt, ScriptBuf, Sequence, Transaction, TxIn, TxOut, Txid, Witness};
use psrgbt::{RgbOutExt, RgbPropKeyExt, RgbPsbtExt};
use rgbcore::commit_verify::{mpc, CommitId, ConvolveCommit, MerkleHash};
use rgbcore::seals::txout::{ChainBlindSeal, SingleBlindSeal};
use rgbcore::tapret::TapretPathProof;
use rgbcore::{ContractId, Operation, Vout};
use rgbstd::containers::{ConsignmentExt, Contract, Transfer};
use rgbstd::containers::BuilderSeal;
use rgbstd::contract::{AllocatedState, TransitionBuilder};
use rgbstd::invoice::RgbInvoice;
use rgbstd::persistence::Stock;
use rgbstd::txout::{CloseMethod, ExplicitSeal};
use rgbstd::validation::{ResolveWitness, ValidationConfig, WitnessOrdProvider, WitnessResolverError, WitnessStatus};
use rgbstd::vm::WitnessOrd;
use rgbstd::{Amount, AssignmentType, ChainNet, GraphSeal, Opout};
use strict_encoding::{StrictEncode, StrictSerialize, StrictWriter};

fn hex(data: &[u8]) -> String {
    const CHARS: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(data.len() * 2);
    for byte in data {
        out.push(CHARS[(byte >> 4) as usize] as char);
        out.push(CHARS[(byte & 0x0f) as usize] as char);
    }
    out
}

fn strict_hex<T: StrictEncode>(value: &T) -> String {
	let writer = StrictWriter::in_memory::<1_000_000>();
	let writer = value.strict_encode(writer).unwrap();
	hex(&writer.unbox().unconfine())
}

fn run_inspector() -> bool {
	let args = env::args().collect::<Vec<_>>();
	if args.len() != 3 {
		return false;
	}
	match args[1].as_str() {
		"inspect-transfer" => {
			let armored = fs::read_to_string(&args[2]).expect("read transfer consignment");
			let transfer = Transfer::from_ascii_armored_str(&armored)
				.expect("official Rust parser rejected transfer consignment");
			println!("{}", serde_json::json!({
				"id": transfer.consignment_id().to_string(),
				"contract_id": transfer.contract_id().to_string(),
				"schema_id": transfer.schema_id().to_string(),
				"canonical_roundtrip": Transfer::from_ascii_armored_str(&transfer.to_string()).is_ok(),
			}));
			true
		}
		"inspect-invoice" => {
			let invoice = RgbInvoice::from_str(&args[2])
				.expect("official Rust parser rejected invoice");
			println!("{}", serde_json::json!({"invoice": invoice.to_string()}));
			true
		}
		_ => false,
	}
}

fn main() {
	if run_inspector() {
		return;
	}
    let mut vectors = BTreeMap::<&str, String>::new();

    let contract = ContractId::from_str(
        "rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E",
    )
    .expect("official contract id vector");
    vectors.insert("contract_id", contract.to_string());
    let txid = Txid::from_str(
        "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100",
    )
    .expect("test txid");
    let seal = SingleBlindSeal::with_blinding(
        txid,
        Vout::from_u32(0x11223344),
        0x0102030405060708,
    );
    let writer = StrictWriter::in_memory::<1024>();
    let writer = seal.strict_encode(writer)
        .expect("strict blind seal serialization");
    let strict = writer.unbox().unconfine();
	vectors.insert("blind_seal_strict_hex", hex(&strict));
    vectors.insert("blind_seal_concealed", seal.to_secret_seal().to_string());
	let witness_seal = ChainBlindSeal::with_blinded_vout(7u32, 0x0102030405060708);
	let writer = StrictWriter::in_memory::<1024>();
	let writer = witness_seal
		.strict_encode(writer)
		.expect("strict witness graph seal serialization");
	let strict = writer.unbox().unconfine();
	vectors.insert("graph_seal_strict_hex", hex(&strict));
	vectors.insert(
		"graph_seal_concealed",
		witness_seal.to_secret_seal().to_string(),
	);

	let transfer = Transfer::from_ascii_armored_str(include_str!(
		"../../../testvectors/rc11/armored_transfer.txt"
	))
	.expect("official armored transfer vector");
	vectors.insert("armored_transfer_id", transfer.consignment_id().to_string());
	vectors.insert(
		"armored_transfer_json",
		serde_json::to_string(&transfer).expect("serialize armored transfer"),
	);
	let nia_contract = Contract::from_ascii_armored_str(include_str!(
		"../../../testvectors/rc11/nia-example.rgba"
	))
	.expect("official NIA contract vector");
	let genesis = nia_contract.genesis();
	vectors.insert("nia_contract_id", genesis.contract_id().to_string());
	vectors.insert("nia_contract_consignment_id_hex", hex(nia_contract.consignment_id().as_slice()));
	vectors.insert("nia_genesis_disclose_hash", genesis.disclose_hash().to_string());
	vectors.insert("nia_genesis_disclose_strict_hex", strict_hex(&genesis.disclose()));
	let genesis_assigns = genesis.assignments_by_type(AssignmentType::with(4000)).expect("NIA genesis owner assignment");
	let genesis_seal = genesis_assigns.revealed_seal_at(0).unwrap().unwrap();
	vectors.insert("nia_genesis_graph_seal_strict_hex", strict_hex(&genesis_seal));
	vectors.insert("nia_genesis_graph_seal_secret", genesis_assigns.confidential_seal_at(0).unwrap().to_string());
	vectors.insert("nia_typesystem_id", nia_contract.types.id().to_string());
	vectors.insert("nia_typesystem_id_hex", hex(nia_contract.types.id().as_slice()));
	vectors.insert("nia_script_ids", nia_contract.scripts.iter().map(|lib| lib.id().to_string()).collect::<Vec<_>>().join(","));
	vectors.insert("nia_script_ids_hex", nia_contract.scripts.iter().map(|lib| hex(lib.id().as_slice())).collect::<Vec<_>>().join(","));
	vectors.insert("nia_schema_id", nia_contract.schema_id().to_string());
	vectors.insert("nia_issuer_hash", hex(genesis.issuer.commit_id().as_slice()));
	vectors.insert("nia_metadata_hash", hex(genesis.metadata.commit_id().as_slice()));
	vectors.insert(
		"nia_globals_root",
		hex(MerkleHash::merklize(&genesis.globals).as_slice()),
	);
	vectors.insert(
		"nia_assignments_root",
		hex(MerkleHash::merklize(&genesis.assignments).as_slice()),
	);

	macro_rules! contract_vectors {
		($prefix:literal, $fixture:literal) => {{
			let contract = Contract::from_ascii_armored_str(include_str!($fixture))
				.expect(concat!("official ", $prefix, " contract vector"));
			let genesis = contract.genesis();
			vectors.insert(concat!($prefix, "_contract_id"), genesis.contract_id().to_string());
			vectors.insert(
				concat!($prefix, "_contract_consignment_id_hex"),
				hex(contract.consignment_id().as_slice()),
			);
			vectors.insert(
				concat!($prefix, "_genesis_disclose_hash"),
				genesis.disclose_hash().to_string(),
			);
			vectors.insert(
				concat!($prefix, "_genesis_disclose_strict_hex"),
				strict_hex(&genesis.disclose()),
			);
			let assignments = genesis
				.assignments_by_type(AssignmentType::with(4000))
				.expect(concat!($prefix, " genesis owner assignment"));
			let seal = assignments.revealed_seal_at(0).unwrap().unwrap();
			vectors.insert(
				concat!($prefix, "_genesis_graph_seal_strict_hex"),
				strict_hex(&seal),
			);
			vectors.insert(
				concat!($prefix, "_genesis_graph_seal_secret"),
				assignments.confidential_seal_at(0).unwrap().to_string(),
			);
			vectors.insert(concat!($prefix, "_typesystem_id"), contract.types.id().to_string());
			vectors.insert(
				concat!($prefix, "_typesystem_id_hex"),
				hex(contract.types.id().as_slice()),
			);
			vectors.insert(
				concat!($prefix, "_script_ids"),
				contract.scripts.iter().map(|lib| lib.id().to_string()).collect::<Vec<_>>().join(","),
			);
			vectors.insert(
				concat!($prefix, "_script_ids_hex"),
				contract.scripts.iter().map(|lib| hex(lib.id().as_slice())).collect::<Vec<_>>().join(","),
			);
			vectors.insert(concat!($prefix, "_schema_id"), contract.schema_id().to_string());
			contract
		}};
	}

	let ifa_contract = contract_vectors!("ifa", "../../../testvectors/rc11/ifa-example.rgba");
	let cfa_contract = contract_vectors!("cfa", "../../../testvectors/rc11/cfa-example.rgba");
	let uda_contract = contract_vectors!("uda", "../../../testvectors/rc11/uda-example.rgba");

	let input = Opout::new(genesis.id(), AssignmentType::with(4000), 0);
	let output_seal = GraphSeal::with_blinded_vout(Vout::from_u32(1), 0x0102030405060708);
	let transition = TransitionBuilder::named_transition(
		nia_contract.contract_id(),
		nia_contract.schema().clone(),
		"transfer",
		nia_contract.types.clone(),
	)
	.expect("NIA transfer builder")
	.add_input(input, AllocatedState::Amount(Amount::from(100_000u64).into()))
	.expect("NIA transfer input")
	.add_fungible_state(
		"assetOwner",
		BuilderSeal::Revealed(output_seal),
		100_000u64,
	)
	.expect("NIA transfer output")
	.set_nonce(42)
	.complete_transition()
	.expect("complete NIA transition");
	vectors.insert("nia_transition_id", transition.id().to_string());
	vectors.insert(
		"nia_transition_strict_hex",
		hex(&transition.to_strict_serialized::<{ usize::MAX }>().unwrap()),
	);
	let confidential_transition = TransitionBuilder::named_transition(
		nia_contract.contract_id(),
		nia_contract.schema().clone(),
		"transfer",
		nia_contract.types.clone(),
	)
	.expect("NIA confidential transfer builder")
	.add_input(input, AllocatedState::Amount(Amount::from(100_000u64).into()))
	.expect("NIA confidential transfer input")
	.add_fungible_state(
		"assetOwner",
		BuilderSeal::Concealed(output_seal.to_secret_seal()),
		100_000u64,
	)
	.expect("NIA confidential transfer output")
	.set_nonce(42)
	.complete_transition()
	.expect("complete NIA confidential transition");
	vectors.insert("nia_confidential_transition_id", confidential_transition.id().to_string());
	vectors.insert("nia_confidential_transition_strict_hex", strict_hex(&confidential_transition));

	macro_rules! fungible_transfer_vectors {
		($prefix:literal, $contract:expr) => {{
			let contract = &$contract;
			let genesis = contract.genesis();
			let assignments = genesis
				.assignments_by_type(AssignmentType::with(4000))
				.expect(concat!($prefix, " owner assignment"));
			let state = *assignments
				.as_fungible_state_at(0)
				.expect(concat!($prefix, " fungible state"));
			let transition = TransitionBuilder::named_transition(
				contract.contract_id(),
				contract.schema().clone(),
				"transfer",
				contract.types.clone(),
			)
			.expect(concat!($prefix, " transfer builder"))
			.add_input(
				Opout::new(genesis.id(), AssignmentType::with(4000), 0),
				AllocatedState::Amount(state),
			)
			.expect(concat!($prefix, " transfer input"))
			.add_fungible_state_raw(
				AssignmentType::with(4000),
				BuilderSeal::Concealed(output_seal.to_secret_seal()),
				state.as_u64(),
			)
			.expect(concat!($prefix, " transfer output"))
			.set_nonce(42)
			.complete_transition()
			.expect(concat!("complete ", $prefix, " transfer"));
			vectors.insert(
				concat!($prefix, "_confidential_transition_id"),
				transition.id().to_string(),
			);
			vectors.insert(
				concat!($prefix, "_confidential_transition_strict_hex"),
				strict_hex(&transition),
			);
		}};
	}

	fungible_transfer_vectors!("ifa", ifa_contract);
	fungible_transfer_vectors!("cfa", cfa_contract);

	let uda_genesis = uda_contract.genesis();
	let uda_assignments = uda_genesis
		.assignments_by_type(AssignmentType::with(4000))
		.expect("UDA owner assignment");
	let uda_state = uda_assignments
		.as_structured_state_at(0)
		.expect("UDA structured state")
		.clone();
	let uda_transition = TransitionBuilder::named_transition(
		uda_contract.contract_id(),
		uda_contract.schema().clone(),
		"transfer",
		uda_contract.types.clone(),
	)
	.expect("UDA transfer builder")
	.add_input(
		Opout::new(uda_genesis.id(), AssignmentType::with(4000), 0),
		AllocatedState::Data(uda_state.clone()),
	)
	.expect("UDA transfer input")
	.add_data_raw(
		AssignmentType::with(4000),
		BuilderSeal::Concealed(output_seal.to_secret_seal()),
		uda_state,
	)
	.expect("UDA transfer output")
	.set_nonce(42)
	.complete_transition()
	.expect("complete UDA transfer");
	vectors.insert(
		"uda_confidential_transition_id",
		uda_transition.id().to_string(),
	);
	vectors.insert(
		"uda_confidential_transition_strict_hex",
		strict_hex(&uda_transition),
	);

	let validation_config = ValidationConfig {
		chain_net: ChainNet::BitcoinTestnet4,
		trusted_typesystem: nia_contract.types.clone(),
		..Default::default()
	};
	struct ReferenceResolver;
	impl ResolveWitness for ReferenceResolver {
		fn resolve_witness(&self, _: rgbstd::Txid) -> Result<WitnessStatus, WitnessResolverError> {
			Ok(WitnessStatus::Unresolved)
		}
		fn check_chain_net(&self, _: ChainNet) -> Result<(), WitnessResolverError> {
			Ok(())
		}
	}
	let valid_contract = nia_contract
		.clone()
		.validate(&ReferenceResolver, &validation_config)
		.expect("validate official NIA contract");
	let mut stock = Stock::in_memory();
	stock
		.import_contract(valid_contract, ReferenceResolver)
		.expect("import official NIA contract");

	let prev_txid = Txid::from_str(
		"14295d5bb1a191cdb6286dc0944df938421e3dfcbf0811353ccac4100c2068c5",
	)
	.unwrap();
	let unsigned_tx = Transaction {
		version: transaction::Version::TWO,
		lock_time: absolute::LockTime::ZERO,
		input: vec![TxIn {
			previous_output: OutPoint { txid: prev_txid, vout: 1 },
			script_sig: ScriptBuf::new(),
			sequence: Sequence::ENABLE_RBF_NO_LOCKTIME,
			witness: Witness::new(),
		}],
		output: vec![
			TxOut { value: BtcAmount::ZERO, script_pubkey: ScriptBuf::from_hex("6a").unwrap() },
			TxOut { value: BtcAmount::from_sat(546), script_pubkey: ScriptBuf::new() },
		],
	};
	let mut psbt = Psbt::from_unsigned_tx(unsigned_tx).unwrap();
	psbt.outputs[0].set_opret_host();
	psbt.outputs[0].set_mpc_entropy(0x1122334455667788).unwrap();
	psbt.push_rgb_transition(transition.clone()).unwrap();
	psbt.set_rgb_close_method(CloseMethod::OpretFirst);
	psbt.set_as_unmodifiable();
	let fascia = psbt.rgb_commit().expect("commit NIA PSBT");
	let witness_id = psbt.get_txid();

	struct TentativeWitness;
	impl WitnessOrdProvider for TentativeWitness {
		fn witness_ord(&self, _: rgbstd::Txid) -> Result<WitnessOrd, WitnessResolverError> {
			Ok(WitnessOrd::Tentative)
		}
	}
	stock.consume_fascia(fascia, TentativeWitness).unwrap();
	let beneficiary = ExplicitSeal::with(witness_id, Vout::from_u32(1));
	let real_transfer = stock
		.transfer(
			nia_contract.contract_id(),
			[beneficiary],
			[],
			[],
			Some(witness_id),
		)
		.expect("create NIA transfer consignment");
	let witness_bundle = real_transfer.bundles.first().unwrap();
	vectors.insert("nia_witness_bundle_disclose_hash", witness_bundle.commit_id().to_string());
	vectors.insert("nia_bundle_id", witness_bundle.bundle.bundle_id().to_string());
	vectors.insert(
		"nia_bundle_strict_hex",
		strict_hex(&witness_bundle.bundle),
	);
	vectors.insert(
		"nia_anchor_json",
		serde_json::to_string(&witness_bundle.anchor).unwrap(),
	);
	vectors.insert("nia_transfer_id", real_transfer.consignment_id().to_string());
	vectors.insert("nia_transfer_armored", real_transfer.to_string());
	vectors.insert("nia_witness_tx_hex", hex(&serialize(&psbt.unsigned_tx)));

	let invoice_text = "rgb:eIbQx5Am-XRDjj01-RM~5eo7-rv2nluD-OnBJRAy-S9~Yfts/\
		XvmU3d4_nQQ8S7oagbXi07x5vjMm7P~ERukQNX6SC4M/BF/bc:utxob:\
		4vm1CX2Z-K8hMo59-e7dgGBS-Jka7mYn-Xe~yP85-yUiHHxr-aVlYa";
	let invoice = RgbInvoice::from_str(invoice_text).expect("official NIA invoice vector");
    vectors.insert("invoice_nia", invoice.to_string());
	let operation_id = rgbcore::OpId::copy_from_slice([
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
	])
	.expect("operation id vector");
	let key = ProprietaryKey::rgb_transition(operation_id);
	vectors.insert("psbt_rgb_transition_key_data_hex", hex(&serialize(&key)));
	let internal_pk = UntweakedPublicKey::from_str(
		"c5f93479093e2b8f724a79844cc10928dd44e9a390b539843fb83fbf842723f3",
	)
	.expect("tapret internal key");
	let commitment = mpc::Commitment::from([8u8; 32]);
	let (output_key, _) = internal_pk
		.convolve_commit(&TapretPathProof::root(0), &commitment)
		.expect("tapret root path");
	vectors.insert("tapret_root_output_key", hex(&output_key.serialize()));

	println!("{}", serde_json::to_string_pretty(&vectors).unwrap());
}
