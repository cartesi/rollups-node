# syntax=docker/dockerfile:1
FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

ENV DEBIAN_FRONTEND=noninteractive

# [Optional] Uncomment this section to install additional OS packages.
RUN <<EOF
    apt-get update
    apt-get install -y --no-install-recommends \
        git \
        build-essential \
        libslirp-dev \
        libboost-all-dev \
        liblua5.4-dev \
        gcc-12-riscv64-linux-gnu \
        libc6-dev-riscv64-cross \
        wget \
        patch \
        && rm -rf /var/lib/apt/lists/*
EOF

# Clone and submodule update
RUN <<EOF
    git clone https://github.com/cartesi/machine-emulator-sdk.git --branch v0.19.0 --recurse-submodules
EOF

# ENV PREFIX /workspace/rollups-node/emulator-install

# RUN <<EOF
#     cd machine-emulator-sdk/emulator
#     make install PREFIX=$PREFIX
# EOF

# [Optional] Uncomment the next lines to use go get to install anything else you need
# USER vscode
# RUN go get -x <your-dependency-or-tool>
# USER root

# [Optional] Uncomment this line to install global node packages.
# RUN su vscode -c "source /usr/local/share/nvm/nvm.sh && npm install -g <your-package-here>" 2>&1

# ARG USERNAME=calindra
# ARG USER_UID=1007
# ARG USER_GID=$USER_UID

# # Create the user
# RUN groupadd --gid $USER_GID $USERNAME \
#     && useradd --uid $USER_UID --gid $USER_GID -m $USERNAME \
#     #
#     # [Optional] Add sudo support. Omit if you don't need to install software after connecting.
#     && apt-get update \
#     && apt-get install -y sudo \
#     && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
#     && chmod 0440 /etc/sudoers.d/$USERNAME

# # ********************************************************
# # * Anything else you want to do like clean up goes here *
# # ********************************************************

# # [Optional] Set the default user. Omit if you want to keep the default as root.
# USER $USERNAME
