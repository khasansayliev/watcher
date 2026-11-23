FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /railbot ./cmd/railbot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /railbot /app/railbot
ENV DATA_DIR=/app/data LISTEN_ADDR=:8080
VOLUME /app/data
EXPOSE 8080
ENTRYPOINT ["/app/railbot"]
