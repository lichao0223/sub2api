# 账号本地功能合并检查清单

## 误删来源与修复范围

以 `v0.2.28`（`808493474`）及 `5975ac7ec` 的父提交为基线，对照当前账号创建、编辑组件及后端配置消费路径。
`5975ac7ec`（2026-10-14，`test: sync merged platform behavior`）误删了以下编辑能力：

| 功能 | 必须一起保留的链路 |
| --- | --- |
| OpenAI 请求时区 | `OpenAIRequestTimezoneField`、表单回填、`extra.openai_request_timezone` 保存；HTTP/WS 请求环境上下文和 Web Search 时区处理 |
| 分时段调度 | `AccountSchedulingEditor`、表单回填、`extra.scheduling_schedule` 保存与关闭；必须在各平台 extra 构造完成后合入 |
| 旧式模型提供商监控 | OpenAI/Anthropic API Key 账号的 `extra.model_provider` 选择、回填与保存；不能用独立厂商平台入口替代已有账号 |
| 火山方舟用量查询凭据 | 原生 `volcengine` Coding 账号和旧式 `model_provider=volcengine` 账号的 AK/SK 编辑；SK 留空必须保留已存凭据 |
| Codex 门票状态 | OpenAI OAuth/Setup Token 编辑弹窗显示 ready/blocked/missing；诊断状态只读，不写入管理更新请求 |

同时修复创建旧式火山监控账号时 AK/SK 被多协议平台条件挡住的问题，以及同一提交引入的 Gemini 显式缓存配置区域重复渲染。

## 已核对但不回退的内容

- 多模态规则的编辑入口、回填和保存已在后续提交恢复；本次增加往返保存及影子账号隐藏的回归测试。
- 请求时区、分时段调度的后端核心文件与基线相比未被删除；时区校验、敏感凭据脱敏和留空保留机制仍在。
- OpenAI 请求时区仍使用既有允许列表（默认 `Asia/Singapore`），不是所有 IANA 时区都能选；分时段调度的时区是另一项配置，不能混为一谈。
- 上游 `c2cca510f` 明确回退了“更多筛选”折叠面板，保留当前内联筛选，不将其误判为本地功能丢失。
- 国产/多协议平台的模式、默认地址和协议规则已转入 provider profile 驱动实现，保留新实现，不恢复旧硬编码列表。

## 后续同步上游时

1. 搜索 `MERGE KEEP`。这些是本地扩展，不要为了贴合上游组件而删除；改动时必须检查创建、编辑、回填、保存和后端消费者。
2. 特别注意全量替换 `extra`：各平台构造器结束后再合入调度配置，不能复活已清理的运行态字段，也不能覆盖其他配额/透传配置。
3. 保持 AK/SK 与推理 API Key 分离。不要将脱敏后的空 SK 写成删除，也不要把只读门票状态写回 API。
4. 执行 `make test-frontend`。账号创建/编辑用例已列在 `FRONTEND_CRITICAL_VITEST` 中，不得通过删除或跳过用例解决冲突。
5. 后端执行 `cd backend && go test -tags=unit ./internal/service -run 'TestNormalizeOpenAIRequest|TestOpenAIRequestTimezone|TestValidateOpenAIRequestTimezone|TestSchedulingSchedule|TestAccountScheduleStatus|TestLegacyCNUsage|TestLegacyGLMQuota'`。

主要回归用例位于 `EditAccountModal.spec.ts` 的 `localFeatures merge regressions`、`CreateAccountModal.spec.ts` 的旧式火山账号创建测试和 `openai_request_locale_test.go`。
注释用于提醒，测试用于发现回归；两者都不能保证未来合并绝不会再次误删。
