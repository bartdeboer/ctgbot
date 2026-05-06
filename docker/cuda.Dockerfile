FROM golang:1.24-bookworm AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge

FROM nvidia/cuda:12.8.0-base-ubuntu22.04

ARG CODEX_VERSION=latest
ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:${PATH}"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=compute,utility,video

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates git curl bash make tar zip unzip jq \
        bubblewrap gcc g++ libc6-dev ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
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

COPY --from=hostbridge-build /usr/local/go /usr/local/go
COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
