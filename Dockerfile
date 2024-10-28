# --- 构建阶段 ---
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

# --- 运行阶段 ---
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/server ./server
RUN mkdir -p /app/logs
EXPOSE 8080 9090
ENTRYPOINT ["/app/server"]
