# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
COPY --from=web /app/admin/dist ./admin/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/fastproxy .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/fastproxy /usr/local/bin/fastproxy
ENV CONFIG=/data/fastproxy.db ADMIN_ADDR=":7788"
VOLUME /data
EXPOSE 7788
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s \
  CMD wget -qO- "http://127.0.0.1:7788/api/session" >/dev/null 2>&1 || exit 1
ENTRYPOINT ["fastproxy"]
