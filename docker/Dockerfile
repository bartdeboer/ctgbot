FROM golang:1.24-alpine AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal/chatcommands ./internal/chatcommands
COPY internal/durationparse ./internal/durationparse
COPY internal/hostbridge ./internal/hostbridge
COPY internal/hostbridgetls ./internal/hostbridgetls
COPY internal/modeluuid ./internal/modeluuid
COPY internal/messenger ./internal/messenger
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge ./cmd/hostbridge

FROM node:22-alpine

ARG CODEX_VERSION=latest

RUN apk add --no-cache ca-certificates git curl libc6-compat \
        bash make tar zip unzip jq go \
    && apk add --no-cache --no-scripts bubblewrap \
    && curl -fsSL https://github.com/openai/codex/releases/latest/download/install.sh -o /tmp/install-codex.sh \
    && chmod +x /tmp/install-codex.sh \
    && CODEX_INSTALL_DIR=/usr/local/bin /tmp/install-codex.sh "${CODEX_VERSION}" \
    && rm -f /tmp/install-codex.sh \
    && codex --version

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
