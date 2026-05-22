FROM node:22-bookworm-slim AS node-runtime

FROM golang:1.24-bookworm

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bubblewrap \
        gawk \
        jq \
        pkg-config \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        unzip \
        zip \
    && rm -rf /var/lib/apt/lists/*

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

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
