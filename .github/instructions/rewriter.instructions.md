---
description: "Use when adding a new AI provider rewriter, modifying identity rewrite logic, or working with CCH hashing. Covers Rewriter interface contract, Registry registration, and provider-specific pitfalls."
applyTo: "ai-gateway/src/internal/rewriter/**"
---
# Rewriter Guidelines

## Rewriter 接口

每个 provider 必须实现三个方法：

```go
type Rewriter interface {
    Name() string
    RewriteBody(body []byte, path string, cfg *config.Config) []byte
    RewriteHeaders(header http.Header, cfg *config.Config, provider config.ProviderConfig) http.Header
}
```

## 注册

在 `init()` 中向全局 Registry 注册：

```go
func init() {
    Register("provider_name", &ProviderRewriter{})
}
```

`Register()` 和 `Get()` 定义在 `rewriter.go` 中。

## 实现清单

- [ ] `RewriteBody`: 解析 JSON → 替换身份字段 → 返回序列化后的 `[]byte`；JSON 解析失败时**原样返回**原始 body
- [ ] `RewriteHeaders`: 删除泄露头 → 注入 API 密钥/OAuth token → 规范化 User-Agent
- [ ] 单元测试: 表驱动，覆盖正常重写 + 非法 JSON 回退 + 边界条件
- [ ] 敏感字段脱敏: 删除 `device_id`、`fingerprint`、`client_info`、`baseUrl` 等泄露字段

## 注意事项

- CCH 哈希盐值 `59cf53e54c78` 硬编码于 `anthropic.go`，必须与上游保持一致
- Gemini 的 API 密钥在 URL query 参数中，不在 header 里
- 使用 `identity.BuildCanonicalEnv()` / `identity.BuildCanonicalProcess()` 构建规范环境
- gzip 压缩/解压由 proxy 层处理，rewriter 收到的 body 已解压
- 不要引入新的外部依赖，使用 `encoding/json` 标准库
