# Docker 17.05 or higher
FROM golang:1.8.3-alpine as builder
COPY . /go/src/github.com/restic/restic
WORKDIR /go/src/github.com/restic/restic
RUN go run build.go

###############################################################################
FROM alpine:3.6

RUN apk add --update --no-cache ca-certificates fuse

COPY --from=builder /go/src/github.com/restic/restic/restic /usr/bin


ENTRYPOINT ["/usr/bin/restic"]
