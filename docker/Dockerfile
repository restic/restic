FROM alpine:3.6

COPY restic /usr/bin

RUN apk add --update --no-cache ca-certificates fuse

ENTRYPOINT ["/usr/bin/restic"]
