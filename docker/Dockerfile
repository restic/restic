FROM golang:1.19-alpine AS builder

WORKDIR /go/src/github.com/restic/restic

# Caching dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go run build.go

FROM alpine:latest AS restic

RUN apk add --update --no-cache ca-certificates fuse openssh-client tzdata

COPY --from=builder /go/src/github.com/restic/restic/restic /usr/bin

ENTRYPOINT ["/usr/bin/restic"]
