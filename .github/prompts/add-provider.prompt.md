---
description: "Scaffold a new AI provider: rewriter implementation, UCI config, LuCI form section, and translation entries"
agent: "agent"
argument-hint: "Provider name, e.g. deepseek"
---
为 AI Gateway 添加一个新的 AI provider（名称由用户指定）。按以下步骤生成所有必要文件和代码：

## 1. Rewriter 实现

在 `ai-gateway/src/internal/rewriter/` 下创建 `{provider}.go`：
- 实现 `Rewriter` 接口: `Name()`, `RewriteBody()`, `RewriteHeaders()`
- 在 `init()` 中调用 `Register()` 注册
- 参考 [openai.go](ai-gateway/src/internal/rewriter/openai.go) 的模式

同时创建 `{provider}_test.go`，使用表驱动测试覆盖：
- 正常身份重写
- 非法 JSON 原样返回
- 头部清理与 API 密钥注入

## 2. UCI 默认配置

在 [ai-gateway.conf](ai-gateway/files/ai-gateway.conf) 中追加 provider 配置块：
```
config provider '{provider}'
    option enabled '0'
    option upstream 'https://{api_domain}'
    list domains '{api_domain}'
    option api_key ''
```

## 3. Config 支持

在 [config.go](ai-gateway/src/internal/config/config.go) 的默认域名映射中添加新 provider 条目。

## 4. LuCI 表单

在 [providers.js](luci-app-ai-gateway/htdocs/luci-static/resources/view/ai-gateway/providers.js) 中添加新 provider 的表单 section，包含 enabled/upstream/domains/api_key 字段。

## 5. 翻译

在 [ai-gateway.po](luci-app-ai-gateway/po/zh_Hans/ai-gateway.po) 中添加新 provider 相关的中文翻译条目。

## 6. 验证

完成后运行 `go build ./cmd/ai-gateway/` 和 `go test ./internal/rewriter/...` 确认编译和测试通过。
