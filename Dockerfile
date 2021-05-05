FROM golang:1.16-alpine as builder
WORKDIR /src
COPY . .
RUN go run build.go

FROM alpine:3.12.6
WORKDIR /app
COPY --from=builder /src/restic /bin/restic
ENTRYPOINT ["restic"]
