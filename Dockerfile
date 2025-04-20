# --- 构建阶段 ---
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN ls -l ./pkg/log
RUN ls -l ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server/main.go

# --- 运行阶段 ---
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server ./server
RUN mkdir -p /app/logs
EXPOSE 8080
ENV SERVER_PORT=8080 \
    LOG_PATH=./logs \
    LOG_LEVEL=info \
    APP_NAME=go-serverhttp-template \
    APP_VERSION=0.0.1
ENTRYPOINT ["/app/server"] 