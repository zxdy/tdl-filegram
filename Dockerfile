# ===== 阶段 1：构建前端 =====
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# ===== 阶段 2：构建后端 =====
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 嵌入前端构建产物（go:embed）
COPY --from=web-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tdl-filegram ./cmd/api/

# ===== 阶段 3：运行镜像（nginx 前端 :8744 + Go 后端 :8743）=====
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata nginx
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=go-builder /app/tdl-filegram .
COPY --from=web-builder /app/web/dist /usr/share/nginx/html
RUN mkdir -p /app/config

COPY docker/nginx.conf /etc/nginx/http.d/default.conf
COPY docker/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# ── 可配置环境变量（docker run -e / compose environment 覆盖）──
ENV DB_PATH=/data/tdl-filegram.db \
    DOWNLOAD_DIR=/downloads \
    DOWNLOAD_THREADS=4 \
    DOWNLOAD_LIMIT=2 \
    TG_APP_ID="" \
    TG_APP_HASH="" \
    TG_DATA_DIR=/data/.tdl \
    TG_NAMESPACE=default \
    TG_POOL_SIZE=8 \
    TG_RECONNECT_TIMEOUT=5m \
    TG_PROXY=""
# 8744 = 前端（nginx），8743 = 后端（Go API）
EXPOSE 8744 8743
VOLUME ["/data", "/downloads"]

ENTRYPOINT ["/app/entrypoint.sh"]
