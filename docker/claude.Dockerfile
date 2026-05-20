FROM golang:1.24-bookworm AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge

FROM ctgbot-claude-base:latest

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
