FROM node:24-alpine AS frontend
WORKDIR /build
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund 2>/dev/null || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/static ./web/static
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VERSION}" -o godump .

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata mariadb-client gzip \
    && mkdir -p /etc/godump /backups \
    && command -v mysqldump >/dev/null || ln -s /usr/bin/mariadb-dump /usr/bin/mysqldump
COPY --from=server /build/godump /usr/local/bin/godump
COPY config.yaml /etc/godump/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/godump"]
CMD ["--config", "/etc/godump/config.yaml"]
