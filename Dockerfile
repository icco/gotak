FROM golang:1.25-alpine

ENV GOPROXY="https://proxy.golang.org"
ENV GO111MODULE="on"
ENV NAT_ENV="production"
ENV PORT="8080"

EXPOSE 8080

WORKDIR /go/src/github.com/icco/gotak
RUN apk add --no-cache git

COPY . .

RUN go build -o /go/bin/server ./cmd/server

CMD ["/go/bin/server"]
