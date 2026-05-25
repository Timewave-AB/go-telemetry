FROM golang:1.26-alpine

# gcc + musl-dev are needed for -race (CGO).
RUN apk --no-cache add gcc musl-dev

WORKDIR /src

ENV CGO_ENABLED=1 \
    GOCACHE=/go-cache/build \
    GOMODCACHE=/go-cache/mod
