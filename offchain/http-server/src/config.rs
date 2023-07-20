// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use clap::{
    value_parser, Arg, Command, CommandFactory, FromArgMatches, Parser,
};

#[derive(Debug, Clone, Parser)]
pub struct HttpServerConfig {
    pub(crate) port: u16,
}

impl HttpServerConfig {
    /// Returns the HTTP server config and the app's config after parsing
    /// it from the command line and/or environment variables.
    ///
    /// The parameter `service` must be a lowercase string that
    /// uses underlines as spaces.
    ///
    /// The parametric type `C` must be a struct that derives `Parser`.
    pub fn parse<C: CommandFactory + FromArgMatches>(
        service: &'static str,
    ) -> (HttpServerConfig, C) {
        let command = <C as CommandFactory>::command();
        let command = add_port_arg(command, service);

        let matches = command.get_matches();
        let http_server_config: HttpServerConfig =
            FromArgMatches::from_arg_matches(&matches).unwrap();
        let inner_config: C =
            FromArgMatches::from_arg_matches(&matches).unwrap();
        (http_server_config, inner_config)
    }
}

fn add_port_arg<S: ToString>(command: Command, service: S) -> Command {
    let service = service.to_string().to_uppercase();
    command.arg(
        Arg::new("port")
            .long("http-server-port")
            .env(format!("{}_HTTP_SERVER_PORT", service))
            .value_parser(value_parser!(u16))
            .default_value("8080"),
    )
}
