FROM golang:1.12-alpine
ENV GO111MODULE=on
EXPOSE 8080
WORKDIR /go/src/github.com/icco/gotak
RUN apk add --no-cache git

COPY . .

RUN go build -o /go/bin/server ./server

CMD ["/go/bin/server"]
