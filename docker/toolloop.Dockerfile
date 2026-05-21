FROM ctgbot-toolloop-base:latest AS build

WORKDIR /src
ARG TARGETARCH
COPY go.mod go.sum ./
COPY cmd/hostbridge ./cmd/hostbridge
COPY cmd/toolloop ./cmd/toolloop
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/hostbridge ./cmd/hostbridge \
    && go build -trimpath -ldflags="-s -w" -o /out/toolloop ./cmd/toolloop

FROM ctgbot-toolloop-base:latest

COPY --from=build /out/hostbridge /usr/bin/hostbridge
COPY --from=build /out/toolloop /usr/bin/toolloop

WORKDIR /workspace
CMD ["tail", "-f", "/dev/null"]
