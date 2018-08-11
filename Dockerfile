FROM golang:1.10-alpine

WORKDIR /go/src/github.com/icco/gotak/
COPY . .

RUN apk add --no-cache git

RUN go install -v ./web

CMD ["/go/bin/web"]
