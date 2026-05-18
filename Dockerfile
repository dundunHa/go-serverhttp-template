# --- 构建阶段 ---
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

# --- 运行阶段 ---
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server ./server
EXPOSE 8080
ENV SERVER_PORT=8080 \
    APP_ENV=prod \
    LOG_LEVEL=info
ENTRYPOINT ["/app/server"] 
