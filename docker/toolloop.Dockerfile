FROM ctgbot-go-node-python-base:latest AS build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/apply_patch ./cmd/apply_patch
COPY cmd/hostbridge ./cmd/hostbridge
COPY cmd/toolloop ./cmd/toolloop
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge \
    && go build -trimpath -ldflags="-s -w" -o /out/apply_patch ./cmd/apply_patch \
    && go build -trimpath -ldflags="-s -w" -o /out/toolloop ./cmd/toolloop

FROM ctgbot-go-node-python-base:latest

COPY --from=build /out/hostbridge /usr/bin/hostbridge
COPY --from=build /out/apply_patch /usr/bin/apply_patch
COPY --from=build /out/toolloop /usr/bin/toolloop

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
