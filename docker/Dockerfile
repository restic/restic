FROM golang:1.23-alpine AS builder

WORKDIR /go/src/github.com/restic/restic

# Caching dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go run build.go

FROM alpine:latest AS restic

RUN apk add --no-cache ca-certificates fuse openssh-client tzdata jq

COPY --from=builder /go/src/github.com/restic/restic/restic /usr/bin

# IO class default is "none"- 0, however busybox ionice `-c0 -n<something>`
# since priority has no meaning for no scheduler.
# Thus the set logic below is necessary.
ENV IONICE_CLASS=
ENV IONICE_PRIORITY=4
ENV NICE=0

ENTRYPOINT ["/bin/sh", "-ec", "\
    set -- nice -n \"${NICE}\" /usr/bin/restic \"$@\";  \
    [ -n \"${IONICE_CLASS}\" ] && set -- ionice -c \"${IONICE_CLASS}\" -n \"${IONICE_PRIORITY}\" \"$@\"; \
    exec \"$@\"; \
    "]