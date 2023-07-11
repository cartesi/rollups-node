// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use std::process::Command;

pub fn build(dockerfile: &str, tag: &str, build_args: &[(&str, &str)]) {
    let build_args: Vec<String> = build_args
        .iter()
        .map(|(key, value)| format!("--build-arg={}={}", key, value))
        .collect();
    let mut args = vec!["build", "-f", dockerfile, "-t", tag];
    for build_arg in build_args.iter() {
        args.push(build_arg);
    }
    args.push(".");
    docker_run(&args);
}

pub fn create(tag: &str) -> String {
    let mut id = docker_run(&["create", tag]);
    id.pop().expect("failed to remove new line");
    String::from_utf8_lossy(&id).to_string()
}

pub fn cp(from: &str, to: &str) {
    docker_run(&["cp", from, to]);
}

pub fn rm(id: &str) {
    docker_run(&["rm", "-v", id]);
}

#[tracing::instrument(level = "trace", skip_all)]
fn docker_run(args: &[&str]) -> Vec<u8> {
    tracing::trace!("running docker command 'docker {}'", args.join(" "));
    let output = Command::new("docker")
        .args(args)
        .output()
        .expect("failed to docker_run docker command");
    assert!(
        output.status.success(),
        "failed to docker_run command 'docker {}'\n{}",
        args.join(" "),
        String::from_utf8_lossy(&output.stderr)
    );
    output.stdout
}
