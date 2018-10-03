FROM alpine:latest

COPY restic /usr/bin

RUN apk add --update --no-cache ca-certificates fuse

ENTRYPOINT ["/usr/bin/restic"]
