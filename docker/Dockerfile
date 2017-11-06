# Docker 17.05 or higher
FROM golang:1.8.3-alpine as builder
COPY . /go/src/github.com/restic/restic
WORKDIR /go/src/github.com/restic/restic
RUN go run build.go

###############################################################################
FROM alpine:3.6

COPY --from=builder /go/src/github.com/restic/restic/restic /usr/bin

RUN apk add --update --no-cache ca-certificates fuse

ENTRYPOINT ["/usr/bin/restic"]
