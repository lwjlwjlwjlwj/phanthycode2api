# 多阶段构建：编译 Go 二进制 + 轻量运行镜像
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/phanthycode2api ./cmd/server

FROM alpine:3.20
WORKDIR /app

# 运行时工具（healthcheck 用 wget）
RUN apk add --no-cache wget

COPY --from=builder /out/phanthycode2api /app/phanthycode2api
COPY config.example.json /app/config.json

# 账号凭证与运行时状态挂载到卷，避免写进镜像层
VOLUME ["/app/auths", "/app/data"]

EXPOSE 7864
CMD ["/app/phanthycode2api", "-config", "/app/config.json"]
