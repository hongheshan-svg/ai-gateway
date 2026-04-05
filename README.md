# AI Gateway for OpenWrt

透明 AI API 网关，运行在 OpenWrt 路由器上，自动规范化连接设备的身份/遥测信息。用 Go 编写，原生适配 OpenWrt 生态。

**GitHub**: https://github.com/hongheshan-svg/ai-gateway

## 功能

- **透明代理** — 通过 DNS 劫持 + TLS MITM 自动拦截 AI API 流量，客户端无需配置代理
- **身份规范化** — 所有设备统一为同一个规范身份（device_id、email、环境指纹等）
- **多提供商支持** — Anthropic (Claude)、OpenAI (ChatGPT)、Google Gemini
- **LuCI 管理界面** — 通过 Web 界面配置提供商、身份、证书
- **UCI 配置** — 原生 OpenWrt 配置系统集成
- **procd 服务** — 自动启动、崩溃重启、配置变更自动重载

## 架构

```
客户端设备                 OpenWrt 路由器                   AI API 服务器
┌──────────┐         ┌─────────────────────┐         ┌──────────────┐
│ Claude   │  DNS    │   dnsmasq           │         │ Anthropic    │
│ Code     │──劫持──▶│ api.anthropic.com   │         │ api.anthropic│
│          │         │   → 路由器 IP        │         │ .com         │
│          │  HTTPS  │                     │  HTTPS  │              │
│          │──────▶  │  ai-gateway (443)   │──────▶  │              │
│          │  (TLS   │  ├ TLS MITM(SNI)    │  (真实  │              │
│          │   MITM) │  ├ 身份替换          │   请求) │              │
│          │         │  └ 转发上游          │         │              │
└──────────┘         └─────────────────────┘         └──────────────┘
                       ▲
                       │  HTTP :8080
                       │  CA 证书下载页面
                       ▼
                     浏览器访问 http://路由器IP:8080
```

## 包结构

```
ai-gateway/                      # OpenWrt 软件包
├── Makefile                     # OpenWrt 包构建 Makefile
├── files/
│   ├── ai-gateway.conf          # UCI 默认配置
│   └── ai-gateway.init          # procd init 脚本
└── src/                         # Go 源码
    ├── go.mod
    ├── cmd/ai-gateway/main.go   # 入口
    └── internal/
        ├── config/              # YAML/UCI 配置读取
        ├── identity/            # 规范身份生成
        ├── logger/              # 日志系统
        ├── proxy/               # HTTPS 反向代理
        ├── rewriter/            # 身份重写引擎
        │   ├── anthropic.go     # Anthropic CCH hash + 完整重写
        │   ├── openai.go        # OpenAI 身份规范化
        │   └── gemini.go        # Gemini 身份规范化
        └── tls/                 # TLS CA 管理 + 动态证书签发

luci-app-ai-gateway/             # LuCI 管理界面
├── Makefile
├── htdocs/luci-static/resources/view/ai-gateway/
│   ├── status.js                # 状态仪表盘
│   ├── providers.js             # 提供商配置
│   ├── identity.js              # 身份配置
│   └── certs.js                 # 证书管理
├── po/zh_Hans/                  # 中文翻译
└── root/usr/share/
    ├── luci/menu.d/             # LuCI 菜单
    └── rpcd/acl.d/              # RPC 权限
```

## 编译安装

### 前置条件

- OpenWrt SDK (与目标路由器版本匹配)
- Git

### 编译步骤

```bash
# 1. 获取 OpenWrt SDK
tar xf openwrt-sdk-*.tar.xz
cd openwrt-sdk-*/

# 2. 将包源码放入 package 目录
cp -r ai-gateway package/
cp -r luci-app-ai-gateway package/

# 3. 更新 feeds
./scripts/feeds update -a
./scripts/feeds install -a

# 4. 编译
make package/ai-gateway/compile V=s
make package/luci-app-ai-gateway/compile V=s

# 5. 生成的 ipk 在
ls bin/packages/*/base/ai-gateway_*.ipk
ls bin/packages/*/base/luci-app-ai-gateway_*.ipk
```

### 安装到路由器

```bash
# 上传到路由器
scp bin/packages/*/base/ai-gateway_*.ipk root@router:/tmp/
scp bin/packages/*/base/luci-app-ai-gateway_*.ipk root@router:/tmp/

# 安装
opkg install /tmp/ai-gateway_*.ipk
opkg install /tmp/luci-app-ai-gateway_*.ipk

# 服务自动启动
/etc/init.d/ai-gateway enable
/etc/init.d/ai-gateway start
```

## 客户端配置

**唯一需要的客户端操作：安装 CA 证书。**

安装后，在浏览器中访问 `http://路由器IP:8080` 查看 CA 证书下载页面和安装指南。

### macOS

```bash
# 下载证书
curl -o /tmp/ai-gateway-ca.crt http://路由器IP:8080/ca.crt

# 安装到系统钥匙串
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /tmp/ai-gateway-ca.crt
```

### Windows (管理员 PowerShell)

```powershell
Invoke-WebRequest -Uri http://路由器IP:8080/ca.der -OutFile $env:TEMP\ai-gateway-ca.der
Import-Certificate -FilePath $env:TEMP\ai-gateway-ca.der -CertStoreLocation Cert:\LocalMachine\Root
```

### Linux

```bash
curl -o /usr/local/share/ca-certificates/ai-gateway-ca.crt http://路由器IP:8080/ca.crt
update-ca-certificates
```

## UCI 配置说明

```bash
# 查看当前配置
uci show ai_gateway

# 启用/禁用
uci set ai_gateway.global.enabled=1

# 配置 Anthropic OAuth token
uci set ai_gateway.anthropic.enabled=1
uci set ai_gateway.anthropic.oauth_access_token='YOUR_TOKEN'

# 配置 OpenAI
uci set ai_gateway.openai.enabled=1
uci set ai_gateway.openai.api_key='sk-YOUR_KEY'

# 应用并重启
uci commit ai_gateway
/etc/init.d/ai-gateway restart
```

## 工作原理

1. **DNS 劫持**: init 脚本向 dnsmasq 添加 `address=/api.anthropic.com/路由器IP` 等规则
2. **TLS MITM**: 运行 ECDSA P-256 Root CA，根据 SNI 动态签发域名证书
3. **身份重写**: 解析 HTTP 请求体 JSON，替换 device_id/email/env 等身份字段
4. **CCH Hash**: 针对 Anthropic，计算 CC Hash（salt + 消息字符 + 版本号 → SHA-256 前3位）
5. **透明转发**: 重写后的请求通过 HTTPS 转发到真实 API 服务器

## 支持的重写

| 提供商    | 身份字段 | 环境遥测 | 系统提示词 | 事件日志 | 请求头 |
|-----------|---------|---------|-----------|---------|--------|
| Anthropic | ✅      | ✅      | ✅        | ✅      | ✅     |
| OpenAI    | ✅      | ✅      | ✅        | -       | ✅     |
| Gemini    | ✅      | ✅      | ✅        | -       | ✅     |

## 隐私安全策略

网关在转发请求前执行以下安全清洗：

**全局（所有提供商）**
- 清除 `X-Forwarded-For`、`X-Real-Ip`、`X-Forwarded-Host` — 防止内网 IP 泄露到上游
- 清除 `Host`、`Connection`、`Proxy-Authorization` 等 hop-by-hop 头

**Anthropic**
- 替换 `metadata.user_id` 中的 device_id/email
- 重写 `<system-reminder>` 块中的环境信息（Platform/Shell/OS/路径）
- 计算并注入 CCH hash（CC版本指纹）
- 清除 `x-anthropic-billing-header` 头和系统提示中的 billing 块
- 重写事件日志批次中的身份/环境/进程指标
- 清除 `baseUrl`、`gateway` 等泄露字段
- 归一化 User-Agent

**OpenAI**
- 将 `user` 字段替换为规范 device_id[:16]
- 删除 `fingerprint`、`device_id`、`client_info` 字段
- 重写 system 消息中的环境信息
- 清除 `x-stainless-*` 遥测头、`x-request-id`
- 清除 `OpenAI-Organization` 和 `OpenAI-Project` 头
- 注入网关 API Key，替换原始 Authorization

**Gemini**
- 删除 `client_info`、`device_id`、`fingerprint`、`metadata` 字段
- 重写 `system_instruction` 和 `contents` 中的环境信息
- 清除所有 `x-goog-*` 遥测头（保留归一化的 `x-goog-api-client`）
- 始终替换 URL 中的 API Key（防止客户端原始 key 泄露）

## 运行测试

```bash
cd ai-gateway/src
go test ./...
```

## 许可证

MIT
