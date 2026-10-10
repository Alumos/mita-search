# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod main.go task_table.go ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/mita-search .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app && adduser -S -u 10001 -G app app
WORKDIR /app
COPY --from=builder /out/mita-search /app/mita-search
COPY config.example.json /app/config.json
COPY static/ /app/static/
COPY knowledge/ /app/knowledge/
ENV LISTEN_PORT=8066 TZ=Asia/Shanghai
USER 10001:10001
EXPOSE 8066
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8066/api/health || exit 1
ENTRYPOINT ["/app/mita-search"]
