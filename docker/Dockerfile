FROM golang:1.24-alpine AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal/hostbridge ./internal/hostbridge
COPY internal/hostbridgetls ./internal/hostbridgetls
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge ./cmd/hostbridge

FROM node:22-alpine

RUN apk add --no-cache ca-certificates git ripgrep curl libc6-compat bubblewrap \
    && npm install -g @openai/codex \
    && npm cache clean --force

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
