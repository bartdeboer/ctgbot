FROM ctgbot-codex-base:latest AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/apply_patch ./cmd/apply_patch
COPY cmd/hostbridge ./cmd/hostbridge
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/hostbridge ./cmd/hostbridge \
    && CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -o /out/apply_patch ./cmd/apply_patch

FROM ctgbot-codex-base:latest

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge
COPY --from=hostbridge-build /out/apply_patch /usr/bin/apply_patch

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
