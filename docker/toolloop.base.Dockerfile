FROM golang:1.24-bookworm AS go-runtime

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH="/usr/local/go/bin:$PATH"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates bash curl git jq ripgrep coreutils sed \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-runtime /usr/local/go /usr/local/go

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
