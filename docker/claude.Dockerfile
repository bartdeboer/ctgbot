FROM ctgbot-claude-base:latest AS hostbridge-build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/apply_patch ./cmd/apply_patch
COPY cmd/hostbridge ./cmd/hostbridge
COPY cmd/tools ./cmd/tools
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge \
    && go build -trimpath -ldflags="-s -w" -o /out/apply_patch ./cmd/apply_patch \
    && go build -trimpath -ldflags="-s -w" -o /out/tools ./cmd/tools

FROM ctgbot-claude-base:latest

COPY --from=hostbridge-build /out/hostbridge /usr/bin/hostbridge
COPY --from=hostbridge-build /out/apply_patch /usr/bin/apply_patch
COPY --from=hostbridge-build /out/tools /usr/bin/tools

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
