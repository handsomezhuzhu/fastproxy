# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/fastproxy .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/fastproxy /usr/local/bin/fastproxy
ENV CONFIG=/data/config.json ADMIN_ADDR=":7788"
VOLUME /data
EXPOSE 7788
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- "http://127.0.0.1:7788/api/session" >/dev/null 2>&1 || exit 1
ENTRYPOINT ["fastproxy"]
