FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件并下载依赖
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o metrics-sidecar .

# 使用精简的镜像
FROM alpine:3.17

# 添加基本工具和SSL证书
RUN apk --no-cache add ca-certificates tzdata

# 从构建阶段复制二进制文件
COPY --from=builder /app/metrics-sidecar /usr/local/bin/

# 设置时区
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 8333

# 运行应用
ENTRYPOINT ["/usr/local/bin/metrics-sidecar"] 