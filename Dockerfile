# 前端构建（平台无关：固定构建平台，多架构镜像不需要 QEMU 模拟 node）
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 后端构建（固定构建平台 + GOARCH 交叉编译，arm64 无需模拟）
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -trimpath \
    -ldflags "-s -w -X github.com/linuxsuren/modbus-hub/internal/version.Version=${VERSION}" \
    -o /out/modbus-hub .

FROM alpine:3.20
COPY --from=build /out/modbus-hub /usr/local/bin/modbus-hub
VOLUME /data
EXPOSE 8080 9000
ENTRYPOINT ["modbus-hub"]
CMD ["-addr", ":8080", "-data", "/data"]
