FROM golang:1.24-bookworm AS build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/hostbridge ./cmd/hostbridge
COPY cmd/toolloop ./cmd/toolloop
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge \
    && go build -trimpath -ldflags="-s -w" -o /out/toolloop ./cmd/toolloop

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates bash curl git jq \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/hostbridge /usr/bin/hostbridge
COPY --from=build /out/toolloop /usr/bin/toolloop

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
