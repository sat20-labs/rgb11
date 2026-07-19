use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::path::PathBuf;

use rgbcore::stl::bp_core_stl;
use rgbstd::stl::{
    aluvm_stl, bitcoin_stl, commit_verify_stl, rgb_commit_stl, rgb_logic_stl, rgb_ops_stl,
};
use strict_types::stl::{std_stl, strict_types_stl};
use strict_types::TypeLib;

fn export(lib: TypeLib, target: &PathBuf) {
    let named_sem_ids = lib
        .types
        .iter()
        .map(|(name, ty)| (name.to_string(), ty.sem_id_named(name)))
        .collect::<BTreeMap<_, _>>();
    let value = serde_json::json!({
        "id": lib.id(),
        "namedSemIds": named_sem_ids,
        "library": lib,
    });
    let path = target.join(format!("{}.json", value["library"]["name"].as_str().unwrap()));
    fs::write(path, serde_json::to_vec_pretty(&value).unwrap()).unwrap();
}

fn main() {
    let target = env::args_os()
        .nth(1)
        .map(PathBuf::from)
        .expect("usage: export_types <output-directory>");
    fs::create_dir_all(&target).unwrap();
    for lib in [
        std_stl(),
        strict_types_stl(),
        bitcoin_stl(),
        commit_verify_stl(),
        bp_core_stl(),
        aluvm_stl(),
        rgb_commit_stl(),
        rgb_logic_stl(),
        rgb_ops_stl(),
    ] {
        export(lib, &target);
    }
}
