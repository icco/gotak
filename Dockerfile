FROM golang:1.10-alpine

WORKDIR /go/src/app
COPY . .

RUN apk add --no-cache git

RUN go get -d -v ./...
RUN go install -v ./web/...
RUN ls -alht *

CMD ["cd /go/src/app/web && web"]
