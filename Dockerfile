FROM golang:1.16-alpine as builder
ARG BUILD_DATETIME
WORKDIR /src
COPY . .
RUN go run build.go

FROM alpine:3.14.2
WORKDIR /app
COPY --from=builder /src/restic /bin/restic
ENTRYPOINT ["restic"]
