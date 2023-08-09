---
name: ⬆️  Dependency bump
about: Checklist for bumping dependencies
title: ''
labels: chore
assignees: ''
---

## 📈 Subtasks

- [ ] Update major versions in `Cargo.toml`.
- [ ] If an update requires major work, create the corresponding issue.
- [ ] Update the dependencies in `Cargo.lock`.
- [ ] Update Cartesi dependencies and update README.
- [ ] Update Rust base docker image.
- [ ] Verify whether everything is working as expected.
