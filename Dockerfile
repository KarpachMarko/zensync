# syntax=docker/dockerfile:1
FROM node:24.13.0-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.0-alpine AS downloader
RUN apk add --no-cache git
WORKDIR /src/gofi
# d-fi v2.3.5, pinned to the release commit rather than a moving tag.
RUN git init && git remote add origin https://github.com/d-fi/GoFi.git && git fetch --depth=1 origin fa5612209bccf3ec64aae518c76bf86dcc63c7b8 && git checkout FETCH_HEAD
COPY integrations/gofi/zensync_progress.go.txt ./internal/dfi/zensync_progress.go
COPY integrations/gofi/zensync_progress_test.go.txt ./internal/dfi/zensync_progress_test.go
# Fail the build if the pinned hook integration point ever changes.
RUN test "$(grep -c 'options.Hooks = terminalDownloadHooks(options.Message)' internal/dfi/download_track.go)" = 1 && sed -i 's/options.Hooks = terminalDownloadHooks(options.Message)/options.Hooks = zenSyncDownloadHooks(options.Message)/' internal/dfi/download_track.go
RUN go test ./internal/dfi -run TestZenSyncProgress -count=1
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/d-fi ./cmd/d-fi

FROM golang:1.26.0-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/zensync ./cmd/zensync

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata ffmpeg && addgroup -g 10001 zensync && adduser -D -H -u 10001 -G zensync zensync && mkdir -p /data /music && chown 10001:10001 /data /music
WORKDIR /app
COPY --from=backend /out/zensync /app/zensync
COPY --from=downloader /out/d-fi /usr/local/bin/d-fi
COPY --from=downloader /src/gofi/LICENSE /usr/share/licenses/d-fi/LICENSE
COPY --from=web /src/web/dist /app/web
ENV ZENSYNC_DATA_DIR=/data ZENSYNC_WEB_DIR=/app/web ZENSYNC_MUSIC_DIR=/music
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/app/zensync", "healthcheck"]
ENTRYPOINT ["/app/zensync"]
