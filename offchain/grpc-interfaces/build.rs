// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .protoc_arg("--experimental_allow_proto3_optional")
        .compile(
            &[
                "../../grpc-interfaces/versioning.proto",
                "../../grpc-interfaces/server-manager.proto",
                "../../grpc-interfaces/state-fold-server.proto",
            ],
            &["../../grpc-interfaces"],
        )?;
    println!("cargo:rerun-if-changed=../../grpc-interfaces/versioning.proto");
    println!(
        "cargo:rerun-if-changed=../../grpc-interfaces/server-manager.proto"
    );
    println!(
        "cargo:rerun-if-changed=../../grpc-interfaces/state-fold-server.proto"
    );
    println!("cargo:rerun-if-changed=build.rs");
    Ok(())
}
