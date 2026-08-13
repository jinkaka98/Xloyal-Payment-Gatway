FROM golang:1.23-alpine AS build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate

FROM alpine:3.21 AS runtime
RUN addgroup -S xloyal && adduser -S -G xloyal xloyal && apk add --no-cache ca-certificates curl
COPY --from=build /out/ /usr/local/bin/
USER xloyal
ENTRYPOINT ["/usr/local/bin/api"]
