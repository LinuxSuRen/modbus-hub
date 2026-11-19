# modbus-hub

<p align="center">
  <img src="logo.svg" width="160" alt="modbus-hub logo" />
</p>

[![CI](https://github.com/linuxsuren/modbus-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/linuxsuren/modbus-hub/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![GitHub](https://img.shields.io/badge/GitHub-linuxsuren%2Fmodbus--hub-181717?logo=github)](https://github.com/linuxsuren/modbus-hub)

自托管的 **Modbus 采集网关**：统一读写 **Modbus TCP** 与 **USB 串口（RS485/RS232）上的 Modbus RTU** 设备，并把采集数据持续转发到 **MQTT** 或 **TCP（JSON 行流）**。

树莓派 / 迷你主机 / NAS 一把梭：Go 单二进制内嵌 Web 管理台，SQLite 存配置，无外部依赖。

- **连接**：Modbus TCP（host:port + 从站号）、串口 RTU（/dev/ttyUSB0、COM3，波特率/校验/停止位可配），系统串口自动枚举下拉选择
- **采集**：按点位配置轮询（FC1 线圈 / FC2 离散输入 / FC3 保持寄存器 / FC4 输入寄存器），支持 uint16/int16/uint32/int32/float32（含字交换）/bool 解码与倍率偏移换算（`原始值 × 倍率 + 偏移`，如温度寄存器 240 × 0.1 = 24℃），可配小数位消除浮点噪声（如 3 × 0.1 收敛为 0.3）
- **转发**：MQTT（QoS1、自动重连）与 TCP JSON 行流（`nc` 即可订阅）
- **手动读写**：管理台直接写线圈（FC5）/ 单寄存器（FC6），单点立即读取
- **串口诊断**：一键探测设备真实波特率（逐档尝试常见档位、真实发帧以 CRC 有效的应答判定，Modbus 异常应答也算链路通）
- **健康状态**：连接异常自动重连（指数退避），总览页实时展示
- **实时值**：管理台 SSE 推流（`GET /api/events`），点位添加后读数即时显示在点位列表，无需刷新

## 快速开始

```bash
cd web && npm install && npm run build && cd ..
go build -o modbus-hub .
./modbus-hub -addr :8080 -data ./data
# 浏览器打开 http://localhost:8080
```

Docker 部署（含串口透传示例）：

```bash
docker compose up -d --build
```

或：

```bash
docker run -d -p 8080:8080 -p 9000:9000 \
  -v ./data:/data \
  --device /dev/ttyUSB0:/dev/ttyUSB0 \
  ghcr.io/linuxsuren/modbus-hub:latest
```

## 发布

打 tag 即自动发布（[release.yml](.github/workflows/release.yml)）：

```bash
git tag v0.1.0 && git push origin v0.1.0
```

- **GitHub Release**：goreleaser 构建 linux/windows/darwin × amd64/arm64 二进制（tar.gz/zip + checksums），版本号注入 `-version`
- **容器镜像**：buildx 多架构（linux/amd64、linux/arm64）推送 GHCR `ghcr.io/<owner>/modbus-hub:<版本>` 与 `:latest`，树莓派可直接用 arm64 镜像

## 使用流程

1. **连接管理** → 新建连接（TCP 填 `ip:502`；USB 填串口设备路径），点「测试」验证可达
2. **点位管理** → 按设备手册配置功能码 / 地址 / 数量 / 数据类型 / 倍率偏移 / 小数位 / 轮询周期
3. **总览** → 查看实时读数与连接健康（3 秒自动刷新）
4. **转发设置** → 启用 MQTT 或 TCP 转发

### MQTT 转发

- 读数发布到 `{前缀}/{连接名}/{点位名}`（如 `modbus/plc1/温度1`），QoS1、不 retain
- payload：`{"pointId":1,"pointName":"温度1","connection":"plc1","value":25.5,"unit":"℃","timestamp":"...","ok":true}`

### TCP 转发

启用后在本机监听（默认 `:9000`），每条读数一行 JSON：

```bash
nc 192.168.1.5 9000
{"pointId":1,"pointName":"温度1","connection":"plc1","value":25.5,"unit":"℃","timestamp":"2026-11-05T16:00:00+08:00","ok":true}
```

## REST API

管理台用的全部接口同样开放给脚本集成：`/api/connections`、`/api/connections/probe-baud`（POST 连接配置，探测串口波特率）、`/api/serial/ports`（GET 枚举系统串口）、`/api/points`、`/api/values`、`/api/events`（GET SSE 实时读数流）、`/api/write`、`/api/sinks/*`（详见代码 `internal/httpserver`）。

> 本工具面向内网/工控场景，未内置鉴权；如需暴露到公网请自行加反向代理鉴权。

## 开发

```bash
make test    # go test ./...
make web     # 前端构建
make         # 前端 + 后端全量构建
```
