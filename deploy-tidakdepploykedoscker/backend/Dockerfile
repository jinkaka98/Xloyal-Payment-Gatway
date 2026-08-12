# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.21 AS api
RUN apk add --no-cache ca-certificates && printf '#!/bin/sh\nwget -q -O /dev/null http://127.0.0.1:8080/v1/health\n' > /healthcheck && chmod +x /healthcheck
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENV HTTP_ADDR=:8080
USER nobody
ENTRYPOINT ["api"]

FROM node:22-alpine AS worker
RUN apk add --no-cache ca-certificates
COPY --from=build /out/worker /usr/local/bin/worker
COPY --chown=node:node backend/tools/camofox_browser_checker.mjs /app/camofox_browser_checker.mjs
ENV CAMOUFOX_CHECKER_CMD="node /app/camofox_browser_checker.mjs"
USER node
ENTRYPOINT ["worker"]

