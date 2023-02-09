FROM golang:1.18-alpine as builder
ARG BUILD_DATETIME
WORKDIR /src
COPY . .
RUN go run build.go

FROM alpine:3.16
RUN adduser -S -s /sbin/nologin restic
RUN apk add libcap
WORKDIR /app
COPY --from=builder /src/restic /bin/restic

# This capability allows restic to read files without root access.
# However, this means that the container will have to run with
# this capability set (ex: --cap-add dac_read_search).
#RUN setcap cap_dac_read_search=+ep /bin/restic
#USER restic
ENTRYPOINT ["restic"]
