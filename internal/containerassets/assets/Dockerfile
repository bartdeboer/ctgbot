FROM golang:1.24-alpine AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY hostbridge-src/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge main.go

FROM node:22-alpine

RUN apk add --no-cache ca-certificates git ripgrep bash libc6-compat \
    && npm install -g @openai/codex \
    && npm cache clean --force

COPY --from=hostbridge-build /out/hostbridge /usr/local/bin/hostbridge
COPY container-init.sh /usr/local/bin/codextgbot-init
RUN chmod +x /usr/local/bin/codextgbot-init

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
