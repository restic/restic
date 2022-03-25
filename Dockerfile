FROM golang:1.16-alpine as builder
ARG BUILD_DATETIME
WORKDIR /src
COPY . .
RUN go run build.go

FROM alpine:3.15
RUN adduser  -S  -s /sbin/nologin restic
RUN apk add libcap
WORKDIR /app
COPY --from=builder /src/restic /bin/restic
RUN setcap cap_dac_read_search=+ep /bin/restic
USER restic
ENTRYPOINT ["restic"]
