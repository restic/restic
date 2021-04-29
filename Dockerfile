FROM golang:1.16-alpine as builder
WORKDIR /src
COPY . .
RUN go run build.go

FROM scratch
WORKDIR /app
COPY --from=builder /src/restic .
ENTRYPOINT ["./restic"]
