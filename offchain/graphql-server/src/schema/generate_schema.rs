// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

// Note: it was impractical to generate schema with build.rs,
// because it is executed before crate is built, and many structures/entities
// from graphql module must be used to generate schema

use juniper::{EmptyMutation, EmptySubscription};
use std::fs::File;
use std::io::Write;

use graphql_server::schema::{Query, Schema};

const GRAPHQL_SCHEMA_FILE: &str = "schema.graphql";

fn main() {
    let schema = Schema::new_with_scalar_value(
        Query {},
        EmptyMutation::new(),
        EmptySubscription::new(),
    );
    let graphql_schema = schema.as_schema_language();
    let mut graphql_schema_file = File::create(GRAPHQL_SCHEMA_FILE).unwrap();
    match write!(graphql_schema_file, "{}", graphql_schema) {
        Ok(_) => {}
        Err(e) => {
            eprintln!("Error writing schema to file {}", e);
        }
    }
}
