FROM golang:1.10-alpine

WORKDIR /go/src/github.com/icco/gotak/
COPY . .

RUN apk add --no-cache git

RUN go get -d -v ./...
RUN go install -v ./...

CMD ["/go/bin/web"]
