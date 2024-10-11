{ pkgs ? import <nixpkgs> {} }:
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    libclang
    libslirp
    lua5_4_compat
    # boost181 
    boost
    wget
    pkg-config
    openssl
  ];
}
