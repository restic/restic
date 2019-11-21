# Build restic binary
FROM golang:1.13 AS builder

WORKDIR /build

# Let's cache modules retrieval - those don't change so often
COPY go.mod go.sum ./
RUN go mod download

# Copy the project and build it
COPY . .
RUN make build

# Build restic docker image
FROM alpine:latest

# Retreive previously build binary
COPY --from=builder /build/restic /usr/bin

# Install dependencies
RUN apk add --update --no-cache ca-certificates fuse openssh-client

ENTRYPOINT ["/usr/bin/restic"]
