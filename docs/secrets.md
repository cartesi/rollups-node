# Secret File Handling

**Applies to:** deployments that pass credentials to the node via `*_FILE`
variables (database credentials, RPC endpoints, signing keys, API tokens),
on Kubernetes or Docker Compose.

## Node expectations

Every value loaded from a `*_FILE` variable is validated at startup, before
use; the node refuses to start on violation. The path must resolve to a
**regular file** — symlinks, directories, FIFOs, and device nodes are
rejected.

Secrets must match one of two canonical forms tailored to common deployment scenarios:

| Deployment        | Ownership                         | Mode             |
| ----------------- | --------------------------------- | -----------------|
| Kubernetes Secret | `root:fsGroup` (the node's group) | `0440`           |
| Compose / host    | the node's user                   | `0400` or `0600` |

Anything else is rejected: world bits, `0440` with a non-root owner, `0440`
whose group is not the node's, and `0400`/`0600` owned by another user. The
owner write bit (`0600`) is accepted in the Compose form so the owner can
rotate the file in place.

Affected variables:

- `CARTESI_AUTH_MNEMONIC_FILE`,
- `CARTESI_AUTH_PRIVATE_KEY_FILE`,
- `CARTESI_BLOCKCHAIN_HTTP_AUTHORIZATION_FILE`,
- `CARTESI_DATABASE_CONNECTION_FILE`,
- `CARTESI_BLOCKCHAIN_HTTP_ENDPOINT_FILE`.

Errors name the file but never its contents:

```text
failed to parse CARTESI_AUTH_MNEMONIC_FILE: secret file "/run/secrets/auth_mnemonic" does not conform with uid/gid/mode rules
```

On non-POSIX platforms only the regular-file check is enforced.

## Kubernetes

Mount the `Secret` as a read-only volume (not env vars), with
`defaultMode: 0440` and a Pod `fsGroup` equal to the node's GID: the
kubelet creates the files root-owned with that group, which is exactly the
canonical form above. `fsGroup` must equal the node user's primary GID
(102 in the stock image) and `runAsUser` must be the node's UID (102).
The check compares the file's group against the process's primary GID,
not its supplementary groups, so a different `fsGroup` (e.g. 65534) is
rejected even though Kubernetes makes the file readable.

## Docker Compose

Compose ignores `uid`/`gid`/`mode` on file-based secrets (it warns and
discards them): the in-container ownership and mode are exactly the **host
file's** numeric UID/GID and mode. The node's startup validation is the only
enforcement point, so the file as seen from the container must already match
the Compose canonical form.

The straightforward workaround is to fix the host file itself — `chown
102:102` and `chmod 0400` (or `0600`).

An option that never modifies host files is a one-shot `secret-init`
service: run as root (only to write the fresh root-owned volume), it copies
the files into a named volume with canonical `102:102` / `0400` identity,
and the node mounts that volume read-only at `/run/secrets`. Host file
ownership and modes do not matter — the copy is normalized; the copy
re-runs on every `up`; `down -v` wipes the volume. `compose.yaml`
implements this pattern:

```yaml
services:
  secret-init:
    image: cartesi/rollups-node:devel
    user: root # required by: chown
    entrypoint:
      - sh
      - -c
      - |
        set -e
        cp /src/* /dst/
        chown 102:102 /dst /dst/*
        chmod 0400 /dst/*
    volumes:
      - ./test/secrets:/src:ro
      - node_secrets:/dst
    network_mode: "none"
    restart: "no"

  node:
    depends_on:
      secret-init:
        condition: service_completed_successfully
    volumes:
      - node_secrets:/run/secrets:ro

volumes:
  node_secrets:
```

## Checklist

- [ ] `*_FILE` secret files match a canonical form: `0400`/`0600` node-owned (Compose) or `0440` root:fsGroup (Kubernetes)
- [ ] Kubernetes: `defaultMode: 0440`, `fsGroup` = node user's primary GID (102), `runAsUser` = node's UID (102)
- [ ] Compose: host files `chown 102:102` / `chmod 0400` (YAML `uid`/`gid`/`mode` are ignored)
