FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
# go.sum repoga commit qilingan bo'lsa, tidy hech narsani o'zgartirmaydi
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /watcher ./cmd/watcher

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /watcher /app/watcher
RUN mkdir -p /app/data && chown app /app/data
USER app
ENV DATA_DIR=/app/data LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/watcher"]
