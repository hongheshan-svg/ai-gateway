---
description: "Use when creating or editing LuCI views, forms, or UI components for AI Gateway. Covers LuCI2 JS patterns, UCI binding, i18n, and translation sync."
applyTo: "luci-app-ai-gateway/**/*.js"
---
# LuCI JS Guidelines

## 视图结构

```javascript
'use strict';
'require view';
'require form';
'require uci';
'require rpc';

return view.extend({
    load: function() {
        return Promise.all([ /* 并行加载多数据源 */ ]);
    },
    render: function(data) {
        // form.Map 或 E() 构建 DOM
    }
});
```

## 关键模式

- **表单绑定**: `new form.Map('ai-gateway', ...)` → UCI 配置名
- **Section 类型**: `form.NamedSection` (固定名) / `form.TypedSection` (动态列表)
- **字段类型**: `form.Flag` (布尔), `form.Value` (文本), `form.ListValue` (下拉), `form.DynamicList` (多值)
- **密码字段**: `o.password = true`
- **只读视图**: 设置 `handleSaveApply: null, handleSave: null, handleReset: null`
- **DOM 构建**: `E('div', { 'class': 'cbi-section' }, [ ... ])` — 不要用 `document.createElement`

## i18n 必须

- 所有用户可见字符串用 `_('...')` 包裹
- 新增/修改字符串后同步更新 `po/zh_Hans/ai-gateway.po`

## UCI 配置块对应

| UCI Section | LuCI Section Name | 用途 |
|-------------|-------------------|------|
| `global` | `'global'` | 服务全局参数 |
| `anthropic` / `openai` / `gemini` | Provider 名 | Provider 配置 |
| `canonical` | `'canonical'` | 规范身份 |

## RPC 调用

```javascript
var callServiceList = rpc.declare({
    object: 'service',
    method: 'list',
    params: ['name'],
    expect: { '': {} }
});
```
