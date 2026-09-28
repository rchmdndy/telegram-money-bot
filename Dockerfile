# syntax=docker/dockerfile:1

# --- build -------------------------------------------------------------------
# Pinned to the toolchain in go.mod so image builds and CI agree.
FROM golang:1.26-bookworm AS build

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 is safe: storage uses modernc.org/sqlite (pure Go).
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/moneybot ./cmd/bot

# --- runtime ------------------------------------------------------------------
# distroless/static ships CA certs and /etc/passwd for the nonroot user, but no
# shell and no /usr/share/zoneinfo — cmd/bot/main.go embeds time/tzdata for that.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/moneybot /usr/local/bin/moneybot

# The SQLite file lives on a volume; the process must be able to write here.
WORKDIR /data
VOLUME ["/data"]

ENV DB_PATH=/data/moneybot.db \
    TZ=Asia/Jakarta \
    LOG_LEVEL=info

USER nonroot:nonroot

# Long polling: no inbound port, so no EXPOSE.
ENTRYPOINT ["/usr/local/bin/moneybot"]
