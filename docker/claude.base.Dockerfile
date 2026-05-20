FROM golang:1.24-bookworm AS go-runtime

FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates git curl bash make tar zip unzip jq build-essential \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code \
    && claude --version

COPY --from=go-runtime /usr/local/go /usr/local/go

RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
