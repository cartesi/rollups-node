#!/usr/bin/env python3
# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)
"""Check synthetic SDK identity faults in a disposable git archive.

Requires Python 3.12, Git, Go, cached modules, and the matching Cartesi SDK.
The faults change Go identity values, not the installed native libraries.
"""

import argparse
import io
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile

PROBE = '''// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)
package advancer
import (
    "errors"
    "testing"
    "github.com/cartesi/rollups-node/internal/model"
)
func TestStartupIdentityRejectsMismatch(t *testing.T) {
    service, err := Create(t.Context(), &CreateInfo{})
    if service != nil || !errors.Is(err, model.ErrIncompatibleStateTransition) {
        t.Fatalf("STARTUP_IDENTITY_ASSERTION: service=%v error=%v", service, err)
    }
}
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("revision", nargs="?", default="HEAD")
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--prefix", type=Path, default=Path("/opt/cartesi" if sys.platform == "darwin" else "/usr"))
    args = parser.parse_args()
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("CARTESI_", "PG"))
           and key not in {"DATABASE_URL", "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"}}
    git = ["git", "-c", f"safe.directory={args.repo.resolve()}", "-C", str(args.repo)]
    revision = subprocess.check_output(git + ["rev-parse", "--verify", args.revision + "^{commit}"], env=env, text=True).strip()
    archive = subprocess.check_output(git + ["archive", "--format=tar", revision], env=env)
    with tempfile.TemporaryDirectory(prefix="rollups-startup-identity-") as directory:
        work = Path(directory)
        with tarfile.open(fileobj=io.BytesIO(archive)) as source:
            source.extractall(work, filter="data")
        temporary = work / ".startup-tmp"
        temporary.mkdir()
        env.update(TMPDIR=str(temporary), CGO_ENABLED="1", CGO_CFLAGS=f"-I{args.prefix}/include",
                   CGO_LDFLAGS=f"-L{args.prefix}/lib", GOPROXY="off", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
        command = ["go", "test", "-race", "-count=1", "-ldflags", f"-r {args.prefix}/lib"]
        subprocess.run(command + ["-run", "^TestLinkedEmulatorIdentityMatchesPins$", "./pkg/machine"],
                       cwd=work, env=env, check=True)
        probe = work / "internal/advancer/startup_identity_control_test.go"
        if probe.exists():
            raise SystemExit("archive already contains the temporary probe path")
        probe.write_text(PROBE)
        paths = {name: work / name for name in (
            "internal/advancer/service.go", "pkg/machine/libcartesi.go", "pkg/emulator/types.go", "pkg/emulator/emulator.go")}
        originals = {name: path.read_text() for name, path in paths.items()}

        def change(name, before, after):
            text = paths[name].read_text()
            if text.count(before) != 1:
                raise SystemExit(f"{name}: expected exactly one mutation target: {before!r}")
            paths[name].write_text(text.replace(before, after))

        def check(label, reject=False):
            result = subprocess.run(command + ["-run", "^TestStartupIdentityRejectsMismatch$", "./internal/advancer"],
                                    cwd=work, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            if reject:
                valid = (result.returncode == 1 and "--- FAIL: TestStartupIdentityRejectsMismatch" in result.stdout
                         and "STARTUP_IDENTITY_ASSERTION:" in result.stdout
                         and "supervisor on advancer service Create is nil" in result.stdout)
            else:
                valid = result.returncode == 0
            if not valid:
                raise SystemExit(f"{label}: invalid control result\n{result.stdout}")
            if reject:
                print(result.stdout, end="")
            print(f"{label}: {'rejected by the startup assertion' if reject else 'PASS'}", flush=True)

        try:
            change("pkg/emulator/types.go", "MarchID uint64 = C.CM_MARCHID", "MarchID uint64 = C.CM_MARCHID + 1")
            check("wrong header")
            guard = ('\tif err = machine.ValidateEmulatorMarchID(); err != nil {\n'
                     '\t\treturn nil, fmt.Errorf("could not validate emulator architecture: %w", err)\n\t}\n')
            change("internal/advancer/service.go", guard, "")
            check("removed startup call", reject=True)
            paths["internal/advancer/service.go"].write_text(originals["internal/advancer/service.go"])
            change("pkg/machine/libcartesi.go", 'model.CheckMarchID("emulator header", emulator.MarchID)',
                   'model.CheckMarchID("emulator header", model.CartesiMachineMarchID)')
            check("header pin compared with itself", reject=True)
            paths["pkg/machine/libcartesi.go"].write_text(originals["pkg/machine/libcartesi.go"])
            paths["pkg/emulator/types.go"].write_text(originals["pkg/emulator/types.go"])
            change("pkg/emulator/emulator.go", "return decoded.Processor.Registers.MarchID, nil",
                   "return decoded.Processor.Registers.MarchID + 1, nil")
            check("wrong runtime")
            change("pkg/machine/libcartesi.go", 'return model.CheckMarchID("linked emulator runtime", marchID)',
                   '_ = marchID\n\treturn model.CheckMarchID("linked emulator runtime", model.CartesiMachineMarchID)')
            check("runtime pin compared with itself", reject=True)
        finally:
            for name, text in originals.items():
                paths[name].write_text(text)
        print(f"startup identity controls passed for {revision}")


if __name__ == "__main__":
    main()
