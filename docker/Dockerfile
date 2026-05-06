FROM golang:1.24-alpine AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge ./cmd/hostbridge

FROM node:22-alpine

ARG CODEX_VERSION=latest

RUN apk add --no-cache ca-certificates git curl libc6-compat \
        bash make tar zip unzip jq go \
    && apk add --no-cache --no-scripts bubblewrap \
    && curl -fsSL https://github.com/openai/codex/releases/latest/download/install.sh -o /tmp/install-codex.sh \
    && chmod +x /tmp/install-codex.sh \
    && if [ -n "${CODEX_VERSION}" ] && [ "${CODEX_VERSION}" != "latest" ]; then \
        CODEX_INSTALL_DIR=/usr/local/bin /tmp/install-codex.sh --release "${CODEX_VERSION}"; \
    else \
        CODEX_INSTALL_DIR=/usr/local/bin /tmp/install-codex.sh; \
    fi \
    && rm -f /tmp/install-codex.sh \
    && mkdir -p /opt/codex \
    && codex_target="$(readlink -f /usr/local/bin/codex)" \
    && cp -a "$(dirname "${codex_target}")/." /opt/codex/ \
    && ln -sf /opt/codex/codex /usr/local/bin/codex \
    && chmod -R a+rX /opt/codex \
    && chmod 755 /usr/local/bin/codex \
    && codex --version

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
