# Project Guidelines — AI Gateway for OpenWrt

透明 AI API 网关，用 Go 编写，运行在 OpenWrt 路由器上。通过 DNS 劫持 + TLS MITM 拦截 AI API 流量并规范化身份信息。

## Architecture

```
ai-gateway/          → OpenWrt 软件包 (Go 后端 + procd init)
  src/internal/
    config/          → UCI + YAML 双配置源
    proxy/           → HTTPS 反向代理 (SNI → provider 路由)
    rewriter/        → 多 provider 身份重写引擎 (接口: Rewriter)
    tls/             → 根 CA 管理 + 按需域名证书签发
    identity/        → 无状态身份/环境生成函数库
    logger/          → 四级日志 + 审计日志

luci-app-ai-gateway/ → LuCI Web 管理界面 (纯客户端 JS)
  htdocs/.../view/ai-gateway/
    status.js        → 只读仪表盘 (Promise.all 多源加载)
    providers.js     → Provider 配置 (form.Map + UCI 绑定)
    identity.js      → 身份配置 (多 section 表单)
    certs.js         → 证书管理 (下载/重新生成)
```

详细架构图和功能描述见 [README.md](../README.md)。

## Code Style

### Go (ai-gateway/src/)
- **Go 1.22**，模块名 `github.com/zhengshan/openwrt-ai-gateway`
- **极简依赖**: 仅 `gopkg.in/yaml.v3`，其余全部标准库
- 错误用 `fmt.Errorf("...: %w", err)` 链式包装
- 并发: `atomic.Int64` 计数器 + `sync.RWMutex` 保护缓存，最小化锁范围
- JSON 操作统一 `encoding/json`，无自定义 marshaler
- 测试: 标准 `testing` 包 + 表驱动测试，无 mock 框架

### LuCI JS (luci-app-ai-gateway/htdocs/)
- LuCI2 客户端 JS: `view.extend()`, `form.Map`, `rpc.declare()`
- 异步加载: `load()` 返回 `Promise.all([...])`
- DOM 构建: `E()` 辅助函数
- 只读视图: `handleSaveApply: null`
- 所有用户可见字符串用 `_()` 包裹以支持 i18n

## Build and Test

```bash
# Go 本地编译 (在 ai-gateway/src/ 目录)
go build -o ai-gateway ./cmd/ai-gateway/

# 运行测试
go test ./internal/...

# OpenWrt 交叉编译 (在 SDK 根目录)
make package/ai-gateway/compile V=s
make package/luci-app-ai-gateway/compile V=s

# 运行 (YAML 配置)
./ai-gateway -config config.yaml

# 运行 (UCI 配置, OpenWrt 环境)
./ai-gateway -uci
```

## Conventions

- **Rewriter 接口**: 添加新 provider 需实现 `Rewriter` 接口 (`Name()`, `RewriteBody()`, `RewriteHeaders()`) 并注册到全局 Registry
- **UCI 配置块**: `global` (服务参数) / `provider.*` (各 provider) / `identity.canonical` (规范身份)
- **DNS 劫持**: init 脚本自动生成 dnsmasq 规则，将 provider 域名解析到路由器 LAN IP
- **CA 证书**: ECDSA P-256 根 CA，域名证书按需签发并内存缓存 (双重检查锁)
- **TLS 密钥权限**: CA 私钥严格 `0600`
- **LuCI 菜单路径**: `admin/services/ai-gateway/{status,providers,identity,certs}`
- **翻译文件**: `po/zh_Hans/ai-gateway.po`，新增 UI 字符串需同步更新

## Pitfalls

- `LoadFromUCI()` 依赖 `uci` 命令行工具，仅 OpenWrt 环境可用；本地开发用 YAML 配置
- Gemini API 密钥在 URL query 参数中，注意日志脱敏
- CCH 哈希盐值 `59cf53e54c78` 硬编码，需与上游保持一致
- SSE 流式响应依赖 `http.Flusher`，修改代理逻辑时勿丢失
- 无 SNI 的 TLS 连接会被拒绝
