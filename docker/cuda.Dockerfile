FROM ctgbot-codex-cuda-base:latest AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/apply_patch ./cmd/apply_patch
COPY cmd/hostbridge ./cmd/hostbridge
COPY cmd/hostbridgev2 ./cmd/hostbridgev2
COPY cmd/ctgbot-supervisor ./cmd/ctgbot-supervisor
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge \
    && go build -trimpath -ldflags="-s -w" -o /out/hostbridgev2 ./cmd/hostbridgev2 \
    && go build -trimpath -ldflags="-s -w" -o /out/apply_patch ./cmd/apply_patch \
    && go build -trimpath -ldflags="-s -w" -o /out/ctgbot-supervisor ./cmd/ctgbot-supervisor

FROM ctgbot-codex-cuda-base:latest

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge
COPY --from=hostbridge-build /out/hostbridgev2 /usr/bin/hostbridgev2
COPY --from=hostbridge-build /out/apply_patch /usr/bin/apply_patch
COPY --from=hostbridge-build /out/ctgbot-supervisor /usr/bin/ctgbot-supervisor

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
