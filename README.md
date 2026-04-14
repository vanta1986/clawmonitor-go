# ClawMonitor Go

OpenClaw 智能监控平台的 Go 语言实现版本，监控服务器资源、OpenClaw 状态和 MiniMax API 使用情况。

**版本:** v2.5

## 功能特性

### 系统资源监控
- CPU 使用率（总计 & 每核心）
- 内存使用情况
- 磁盘空间
- 网络 I/O
- 系统进程数
- 系统运行时间

### OpenClaw 状态
- Gateway 运行状态
- Agent 数量和配置
- 已安装 Skills 列表
- 已安装 Extensions
- 会话数量

### MiniMax API 统计
- 调用次数统计
- Token 消耗（输入/输出）
- 成本估算
- Token Plan 配额查询
- 按模型分类统计

### Claude Code & Hermes 状态
- Hermes Agent 信息
- Claude Code 状态
- Gateway 运行状态

## 技术栈

- **语言:** Go 1.22+
- **数据库:** SQLite3
- **系统监控:** github.com/shirou/gopsutil/v4
- **API:** 原生 net/http

## 快速开始

### 编译

```bash
go mod download
go build -o clawmonitor ./cmd/server
```

### 运行

```bash
# 默认端口 8899
./clawmonitor

# 自定义端口
./clawmonitor -port 8080
```

### Docker 运行

```bash
docker build -t clawmonitor .
docker run -p 8899:8899 clawmonitor
```

## API 接口

| 接口 | 说明 |
|------|------|
| `GET /api/health` | 健康检查 |
| `GET /api/version` | 版本信息 |
| `GET /api/status` | 总览（系统 + OpenClaw） |
| `GET /api/system` | 系统资源详情 |
| `GET /api/openclaw` | OpenClaw 状态详情 |
| `GET /api/minimax` | MiniMax 使用统计 |
| `GET /api/external` | Claude/Hermes 状态 |
| `GET /api/history` | 历史数据查询 |

### 查询参数

- `hours` - 历史数据时间范围（小时），默认 24
- `limit` - 返回记录数，默认 100

示例：

```bash
curl http://localhost:8899/api/status
curl http://localhost:8899/api/history?hours=48&limit=50
```

## 项目结构

```
clawmonitor-go/
├── cmd/server/          # 主入口
├── internal/
│   ├── collector/       # 数据采集
│   │   ├── system.go    # 系统资源
│   │   ├── openclaw.go  # OpenClaw 状态
│   │   ├── minimax.go   # MiniMax 统计
│   │   └── external.go  # Claude/Hermes
│   ├── handler/          # API 处理器
│   ├── cache/           # 内存缓存
│   └── storage/         # SQLite 存储
├── go.mod
└── README.md
```

## 性能对比

| 指标 | Python 版本 | Go 版本 |
|------|-------------|---------|
| 启动时间 | ~2-3s | <100ms |
| 内存占用 | ~40MB | ~15MB |
| 二进制大小 | N/A | ~14MB |

## 与 Python 版本对比

| 特性 | Python 版本 | Go 版本 |
|------|-----------|---------|
| 系统资源 | ✅ | ✅ |
| OpenClaw | ✅ | ✅ |
| MiniMax | ✅ | ✅ |
| Claude/Hermes | ✅ | ✅ |
| SQLite 存储 | ✅ | ✅ |
| REST API | ✅ | ✅ |
| 内存缓存 | ✅ | ✅ |
| Web 前端 | ✅ | ❌ |

**注意：** Go 版本目前不包含 Web 前端界面（Web UI），仅提供 REST API。

## License

MIT
