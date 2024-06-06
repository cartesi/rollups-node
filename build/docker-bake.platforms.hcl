# (c) Cartesi and individual authors (see AUTHORS)
# SPDX-License-Identifier: Apache-2.0 (see LICENSE)

target "docker-platforms" {
    platforms = [
        "linux/amd64",
        # TODO: libarchive13 (required by xgenext2fs in the emulator-devel
        # stage) is not available for arm64. We are temporarily disabling this
        # platform now, but will come back to it before merging next/2.0 into
        # main.
        # "linux/arm64"
    ]
}
