# 前端构建
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 后端构建
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/modbus-hub .

FROM alpine:3.20
COPY --from=build /out/modbus-hub /usr/local/bin/modbus-hub
VOLUME /data
EXPOSE 8080 9000
ENTRYPOINT ["modbus-hub"]
CMD ["-addr", ":8080", "-data", "/data"]
