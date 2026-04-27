# Build stage
FROM golang:1.26-alpine AS builder

ENV GOPROXY="https://proxy.golang.org"
ENV CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.23

LABEL org.opencontainers.image.source=https://github.com/icco/gotak
LABEL org.opencontainers.image.description="A Tak server"
LABEL org.opencontainers.image.licenses=MIT

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -S -u 1001 app

WORKDIR /app
COPY --from=builder --chown=app /server .

USER app

ENV NAT_ENV="production"
ENV PORT="8080"
EXPOSE 8080

ENTRYPOINT ["/app/server"]
