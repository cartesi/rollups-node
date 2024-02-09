---
name: ⬆️  Dependency bump
about: Checklist for bumping dependencies
title: ''
labels: chore
assignees: ''
---

## 📈 Subtasks

- [ ] Cartesi
    - [ ] Bump Machine Emulator SDK
    - [ ] Bump Server Manager
    - [ ] Bump Rollups Contracts
    - [ ] Update README
- [ ] Rust
    - [ ] Bump Node version in `Cargo.toml`.
    - [ ] Bump major versions in dependencies in `Cargo.toml`.
    - [ ] Bump minor/patch dependencies with `cargo update`.
    - [ ] Bump Rust version in Docker image.
- [ ] Go
    - [ ] Bump major versions in dependencies in `go.mod`.
    - [ ] Bump minor/patch dependencies with `go get -u all`.
    - [ ] Bump Go version in `go.mod`.
    - [ ] Bump Go version in Docker image.
- [ ] Verify whether everything is working as expected.
