FROM ctgbot-codex-base:latest AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge ./cmd/hostbridge

FROM ctgbot-codex-base:latest

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
