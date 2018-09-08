FROM golang:1.11-alpine
ENV GO111MODULE=on
EXPOSE 8080
WORKDIR /go/src/github.com/icco/gotak/
COPY . .

RUN apk add --no-cache git

RUN go build -o /go/bin/web -v ./web

CMD ["/go/bin/web"]
