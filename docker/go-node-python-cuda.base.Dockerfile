FROM golang:1.24-bookworm AS go-runtime
FROM node:22-bookworm-slim AS node-runtime

FROM nvidia/cuda:12.8.0-base-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive
ARG CTGBOT_UID=1000
ARG CTGBOT_GID=1000
ARG SUPERVISOR_VERSION=v0.0.5
ENV PATH="/usr/local/go/bin:${PATH}"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility,video

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        bash \
        bubblewrap \
        gawk \
        build-essential \
        coreutils \
        curl \
        findutils \
        git \
        grep \
        jq \
        make \
        pkg-config \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        sudo \
        sed \
        tar \
        unzip \
        zip \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --gid ${CTGBOT_GID} ctgbot \
    && useradd --uid ${CTGBOT_UID} --gid ${CTGBOT_GID} --create-home --shell /bin/bash ctgbot \
    && echo 'ctgbot ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ctgbot \
    && chmod 0440 /etc/sudoers.d/ctgbot

COPY --from=go-runtime /usr/local/go /usr/local/go
COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/bin/corepack /usr/local/bin/corepack
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf /usr/local/go/bin/go /usr/local/bin/go \
    && ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && go version \
    && node --version \
    && npm --version \
    && python3 --version \
    && git --version \
    && rg --version >/dev/null

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN CGO_ENABLED=0 GOBIN=/usr/local/bin go install \
        -trimpath \
        -buildvcs=false \
        -ldflags="-s -w -buildid=" \
        github.com/bartdeboer/go-supervisor/cmd/supervisor@${SUPERVISOR_VERSION} \
    && CGO_ENABLED=0 GOBIN=/usr/local/bin go install \
        -trimpath \
        -buildvcs=false \
        -ldflags="-s -w -buildid=" \
        github.com/bartdeboer/go-supervisor/cmd/supervisord@${SUPERVISOR_VERSION}

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
