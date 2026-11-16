# 2026-11-16 上游同步记录

## 合并范围

- 二开基线：`7eac2500e`（`main`），备份分支：`backup/pre-upstream-20261116`。
- 上游：`https://github.com/Wei-Shaw/sub2api.git`，`main`，截止提交 `9cd1a1eb21e84707685e1998dcb5a3d1ceafae49`；合并前再次查询远程确认该提交仍为上游最新。
- 共同祖先：`10ba8f9b4dc2e6023d4a9879ab77b21841babd10`。
- 本次纳入 **179 个上游提交**：**102 个非合并提交、77 个合并提交**。179 是提交图差集，不能视作 179 项独立功能。
- 二开侧有 880 个上游没有的提交。使用双亲 merge 保留两侧历史，未通过 reset、rebase 或整文件覆盖放弃二开。
- 版本保留二开原有 `0.2.64`，未被上游 `0.2.21` 覆盖。

## 上游主要更新

| 方向 | 更新内容及影响 | 代表提交 |
| --- | --- | --- |
| OpenAI/Codex 流式稳定性 | 首段内容前转发 keepalive；队列心跳后仍正确发送 SSE 错误；兼容入口在首次内容前正确换号；输出后出现错误仍执行账号限流等副作用；首段输出临时文件创建失败时回退内存；修复断开后粘性会话回写。 | `c1c9d195b`、`1280006a1`、`38eb97bd7`、`975b2807d`、`82cd13d1e`、`733722f8c` |
| Codex 上下文与工具 | HTTP Responses 清洗额外上下文；保留并规范 additional_tools 声明 ID；子会话沿用父会话 prompt cache key；保留加密函数调用内容；处理 Python re 不支持的 JSON Schema 正则，父级 schema 修改可覆盖嵌套修改。 | `a0a03be96`、`fd6487758`、`8dcdfff22`、`74ba91aa6`、`4584dcb39`、`dd6329b98`、`6613ced57` |
| 账号选择与白名单 | OpenAI 透传账号新增显式模型过滤开关，默认仍忽略历史映射；确保开关及能力字段进入调度快照；白名单拒绝通配符并提示修正。 | `9d5fb150b`、`121a4cca1`、`cca745e8b`、`97705d147` |
| 配额与监控 | OpenAI 刷新失败保留上次观测时间与数值，区分未知额度，监控不因刷新失败直接标错；国产配额随账号快照更新；Kimi 支持新月度套餐并清理缺席窗口；Anthropic 跨 5h 重置保留周额度，读取 Fable scoped limits，并在周额度恢复后解除模型限流；Gemini 刷新时保留已有展示。 | `e67746254`、`bf3951a59`、`849aa593d`、`c54ebd4f1`、`694d09379`、`c829317fc`、`c115dc27a`、`4114fedd1`、`177243016` |
| Gemini/Vertex 新模型与区域 | 新增 Gemini 4 Carbon 支持及优惠期价格规则，补齐 Hydrogen 计费；Vertex 支持 us/eu 多区域端点和不支持模型回退 global；实际多区域请求默认价加价 10%，管理员分组/渠道定价不重复加价。 | `d70a23f84`、`32e70f3d4`、`b32ad6dfe`、`68071ddde`、`2164ea487` |
| Gemini/Antigravity 协议 | 修复同名并行工具混淆、碎片参数最终快照丢失；Gemini chat 入口转发 response_format；原生入口按稳定前缀生成粘性哈希，换号时按真实上游用量处理强制缓存计费；修复 thoughts 流、首内容超时起点和空流释放粘性账号；schema 数组 items 与类型匹配。 | `b0d60a8c8`、`27a61be4c`、`ce026a2cb`、`5846bfbbf`、`b6c21cd5e`、`8d57f461c`、`e055d7baf`、`5cbcb1b59`、`8480cb0cd` |
| Anthropic/Bedrock 与兼容入口 | Claude 并发刷新令牌后可恢复请求；兼容入口遵循 effort 策略并保留 output config/per-turn beta；记录原生 Anthropic 端点及真实响应模型；会话中途 system 转为 Responses developer 消息；Chat Completions → Anthropic 保留 metadata.user_id；Bedrock 按错误语义分类并保留中断 usage，渠道兼容开关保存为 boolean；默认账号测试模型改 Sonnet 4.6。 | `b0f6f8391`、`d850b8e3a`、`bd89d3d94`、`6d7e6b9d8`、`6c79fdfc7`、`1f2242436`、`23ac69a02`、`5c96ee4d2`、`734b922bd` |
| Grok/复合路由/搜索 | 保留 xAI 顶层媒体错误消息与代码；服务端搜索不混入客户端工具调用；复合路由响应还原公开模型且仅在 message_start 改写；账号搜索模式在支持平台一致生效；count_tokens 不再重复转发请求头。 | `c8f4264f8`、`03a489e43`、`70ad8365d`、`aee71b85b`、`7349750c0`、`a0340d4dd`、`2b86d2f6a` |
| 性能、扣费与汇总 | 按用户通道合并结算，减少热点余额行排队；请求 JSON 热路径减少解码和复制；调度快照与数据库回源按 bucket 合并；冷却中的账号仍保留快照成员身份，过期后可恢复；分组日汇总分阶段登记/计算/发布，保留期清理与重建减少对 usage_logs 写入的阻塞；分区删锁等待超时后回退行清理。 | `2101f4f94`、`ade20f4e4`、`71ef57705`、`1546c0c18`、`9c3280098`、`39bac35a3`、`829bf2cdb` |
| 统计、定价、支付及运维 | 账号筛选下实际扣费与账号成本分开返回/展示；DeepSeek 尊重显式价格覆盖；分组费率/RPM 同步使用事务，RPM override 按组生效；支付宝退款重试沿用稳定请求 ID；价格文件启动检查改后台；上游 TCP 保活约 30s 发现死连接，设置 TCP_USER_TIMEOUT 失败时不阻断连接；禁用并发等待 ping；注入设置的 HTML 不缓存；登录邮箱身份回填失败不再 panic；Go 1.27.4 安全升级（本二开已提前升级），另有赞助信息更新。 | `d05e8ef03`、`b7878328b`、`98a44ba3f`、`37a0f3870`、`43511b40e`、`c45ef4116`、`5854a1db4`、`94e56dd19`、`fe40612b8`、`f1394a1ab`、`197109ebc`、`ddb7eac63` |

## 11 个冲突文件及处理依据

| 冲突文件 | 冲突原因 | 最终处理 |
| --- | --- | --- |
| `backend/cmd/server/VERSION` | 两侧版本序列独立 | 保留二开 `0.2.64`。 |
| `backend/cmd/server/wire_gen.go` | 上游新增按用户扣费合并器，二开有非工作日汇总和 WorkInsight 生命周期 | 同时保留两侧依赖与清理，使用 Wire 重新生成，与 `wire.go` 对齐。 |
| `backend/internal/repository/dashboard_aggregation_repo.go` | 上游减少汇总清理锁阻塞，二开需要核算完成后归档被删日志 | 保留上游整日保留边界、非阻塞汇总、3s 分区锁超时及回退行删除；事务内继续使用二开核算 advisory lock，核算激活后非空分区禁止直接 DROP，先经父表 DELETE 触发归档，再删除空分区。 |
| `backend/internal/repository/dashboard_aggregation_group_usage_test.go` | 两侧清理事务/SQL 预期不同 | 适配新的锁顺序及 SQL，保留二开非空源保护测试，新增核算忙与已归档空分区测试，同时保留上游保留边界、锁超时与失败回滚测试。 |
| `backend/internal/repository/integration_harness_test.go` | 两侧给数据库 DSN 增加不同变量名 | 同时保留并赋值，兼容二开核算与上游汇总测试。 |
| `backend/internal/service/account_scheduling_threshold_eval.go` | 二开已加火山月度窗口，上游新增 Kimi 月度窗口 | 取功能并集，保留 Volcengine、OpenCode Go、Command Code 并加入 Kimi，增加月度候选回归用例。 |
| `frontend/src/components/account/AccountUsageCell.vue` | 二开增加门票与厂商展示，上游改额度新鲜度与并发刷新 | 保留 OAuth/Setup Token 门票与本地厂商功能，新增刷新失败提示、观测时间、未知额度；保留上游卸载/切换账号后的异步回写保护。移除因二开已替换相关判断而不再使用的导入。 |
| `frontend/src/components/account/CreateAccountModal.vue` | 时区字段与透传模型过滤字段在相同位置插入 | 同时保留两项状态、重置与序列化；保留二开调度、旧式厂商 AK/SK、多模态链路。 |
| `frontend/src/components/account/EditAccountModal.vue` | 时区/分时调度与透传模型过滤状态插入重叠 | 保留创建/回填/保存对应字段，调度仍在各平台 extra 构造完成后合入。 |
| `frontend/src/components/account/__tests__/AccountUsageCell.spec.ts` | 两侧新增导入和 mock | 合并类型与厂商 mock，保留全部用例，增加门票与失败提示共存测试。 |
| `frontend/src/components/account/__tests__/EditAccountModal.spec.ts` | 本地功能回归块与上游透传过滤测试插在同一位置 | 两组测试全部保留；透传过滤测试同时断言时区、调度、分组和模型映射均保存。 |

额外处理自动合并未提示的兼容点：

- 上游新增两个 handler 测试缺少二开 `NewOpenAIGatewayService` 的额外构造参数，已补齐测试依赖。
- 上游汇总集成测试的隔离 schema 需要二开的 analytics 激活状态表，已补齐 fixture，避免测试误用公共 schema 或查询不存在的表。
- 二开核算、WorkInsight、请求时区核心文件未被上游替换；`MERGE KEEP` 创建/编辑/保存链路及原有本地功能回归用例保留。
- 新增迁移 `251_group_usage_rollup_nonblocking_build.sql`，与二开 `9xx` 核算迁移文件共存，未重写既有迁移内容。

## 测试及 GitHub Actions

按用户要求，测试由 GitHub Actions 执行，本地不继续运行测试；这里记录覆盖范围，不表示 CI 已通过。

新增/加强的单元回归：

- PostgreSQL 分区删除：活跃核算时保留非空分区、核算锁占用时返回忙、完成归档后允许删除空分区。
- 月度阈值候选：火山方舟与新增 Kimi/OpenCode Go 窗口并存。
- 前端：时区/分时调度与透传模型过滤一起保存，门票与失败刷新/缓存用量同时可见。

新增 PostgreSQL 集成测试 `analytics_partition_retention_integration_test.go`：

1. 在隔离 schema 中同时安装真实核算触发器与上游汇总迁移。
2. 已激活核算的非空分区保留；未完成核算时行删除失败且源日志回滚保留。
3. 完成核算后按行清理，检查 1000 input token、1 次请求归档及 archived_count，随后删除空分区；再次清理不重复归档。
4. 另一个连接持有核算锁时返回忙，不删除源数据；释放后可重试。

CI 配置：

- 原有 `make test-unit`、`make test-integration` 覆盖后端，集成测试使用 Testcontainers 启动 PostgreSQL/Redis。
- 增加相关单元测试竞态检查，覆盖分区保护、月度窗口与按用户合并扣费。
- 对新增的核算保留期集成测试增加 `-race` 执行。
- `make test-frontend` 继续包含 lint、typecheck 和关键 Vitest，补入 AccountUsageCell、ModelWhitelistSelector、ChannelsView 的相关用例。
- Workflow：`.github/workflows/backend-ci.yml`；远程结果以 [GitHub Actions](https://github.com/lichao0223/sub2api/actions/workflows/backend-ci.yml) 为准。

本地阶段：Wire 已重新生成，Go 文件已 gofmt，静态差异检查无空白错误。用户要求转交 CI 前启动的检查曾发现 handler 测试构造参数和未使用导入问题，已修复；完整测试未在本地验证通过。

## 全部非合并提交（102 个）

以下保留原始标题，便于从主题总结回溯到具体改动；按拓扑顺序由旧到新排列。

- [`6dd5a045b`](https://github.com/Wei-Shaw/sub2api/commit/6dd5a045b62e27e5b5b139adef91a171097f639b) chore: sync VERSION to 0.2.21 [skip ci]
- [`530c5e3a4`](https://github.com/Wei-Shaw/sub2api/commit/530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484) chore: update sponsors
- [`a29321497`](https://github.com/Wei-Shaw/sub2api/commit/a293214977a5536fed44b50e46a39e37032ec7bd) chore(ci): bump Go toolchain to 1.27.4 for GO-2026-6859
- [`c115dc27a`](https://github.com/Wei-Shaw/sub2api/commit/c115dc27ab2e21e121874555b630ba77348badcf) fix(usage): read Fable weekly quotas from scoped limits
- [`43511b40e`](https://github.com/Wei-Shaw/sub2api/commit/43511b40e977060ba173b68dbb6fd1e89d2fe578) fix(payment): make Alipay refund request id stable across retries
- [`37a0f3870`](https://github.com/Wei-Shaw/sub2api/commit/37a0f3870b5a269b2957987d86bc76022710bca1) fix(billing): scope RPM override to the group it belongs to
- [`1546c0c18`](https://github.com/Wei-Shaw/sub2api/commit/1546c0c18e97225cc657444ea8406d1088ebd740) perf(gateway): skip JSON decode for forwarded Anthropic stream events
- [`98a44ba3f`](https://github.com/Wei-Shaw/sub2api/commit/98a44ba3f36f3acfd77cebc796fc10248c582018) fix(billing): make group rate/RPM sync transactional
- [`5846bfbbf`](https://github.com/Wei-Shaw/sub2api/commit/5846bfbbfaccb3bfad8f3bc006e2619306aaadb4) fix(gemini): 兼容入口 chat 请求转发 response_format 结构化输出
- [`74ba91aa6`](https://github.com/Wei-Shaw/sub2api/commit/74ba91aa6b6d130d0b0029fabffed6ce3425dffe) fix(openai): keep Codex forks on their parent's prompt cache key
- [`71ef57705`](https://github.com/Wei-Shaw/sub2api/commit/71ef5770547e16007ea0c41ad94f43698d091d59) perf(gateway): trim per-request JSON work on the Anthropic request path
- [`a8481f0de`](https://github.com/Wei-Shaw/sub2api/commit/a8481f0dec1bd319a1cdefec79363ccf7ee8ce2e) test(gateway): satisfy errcheck in request JSON tests
- [`e055d7baf`](https://github.com/Wei-Shaw/sub2api/commit/e055d7bafc5b7eda444e0ec7c7aed99e2c802de1) fix(antigravity): stream Gemini compat thoughts and anchor pre-content timeout after headers
- [`23ac69a02`](https://github.com/Wei-Shaw/sub2api/commit/23ac69a0219c77e29de7b8f4195a40ccc745a7b4) fix(bedrock): 按错误语义区分封号、用例缺失、限流与内容过滤，流中断保留 usage
- [`fe40612b8`](https://github.com/Wei-Shaw/sub2api/commit/fe40612b804dc848be85fcf677bb4e23824918d8) fix: allow disabling concurrency wait pings
- [`4584dcb39`](https://github.com/Wei-Shaw/sub2api/commit/4584dcb39228ec606193f63d79b7a465e8751c1e) fix(apicompat): retain encrypted function call content
- [`197109ebc`](https://github.com/Wei-Shaw/sub2api/commit/197109ebce6ab4700052aad261b7200574b8932e) fix(auth): 登录回填 email 身份失败时不再 panic
- [`829bf2cdb`](https://github.com/Wei-Shaw/sub2api/commit/829bf2cdb9a6a235a5d447ccd92405dbbc123577) fix(usage): 分组日汇总重建与保留期清理不阻塞 usage_logs 写入
- [`fa910c1c3`](https://github.com/Wei-Shaw/sub2api/commit/fa910c1c3842a627a3b9372bf73e7cee11b26593) fix(usage): align five-hour window start with active quota resets
- [`6c79fdfc7`](https://github.com/Wei-Shaw/sub2api/commit/6c79fdfc7ab55e2c067b475ee4a498efd12423c2) fix(apicompat): send mid-conversation system messages to Responses as developer
- [`c829317fc`](https://github.com/Wei-Shaw/sub2api/commit/c829317fc5987b5d0608c861230b45325dd70d31) fix(anthropic): keep weekly usage readings across 5h rollovers
- [`733722f8c`](https://github.com/Wei-Shaw/sub2api/commit/733722f8c7de5f459a490957c36344cb8e20b019) fix(gateway): retain sticky writes after client disconnects
- [`d30179f4b`](https://github.com/Wei-Shaw/sub2api/commit/d30179f4b5c8f56750fea3d6a7a3f74d50796230) fix(usage): retain native Anthropic response observations
- [`ac981240c`](https://github.com/Wei-Shaw/sub2api/commit/ac981240cc44f4e61bf76aa1b67eb1a5436aab30) fix(usage): retain response models through Chat fallbacks
- [`636981b28`](https://github.com/Wei-Shaw/sub2api/commit/636981b28b22fb217222035617c77e7cf7adbe60) test(scheduler): reproduce cooldown expiry absent from snapshots
- [`39bac35a3`](https://github.com/Wei-Shaw/sub2api/commit/39bac35a374f53264a353b69ed21661de9849971) fix(scheduler): retain cooling accounts in snapshot membership
- [`203de719c`](https://github.com/Wei-Shaw/sub2api/commit/203de719c3359936579eaff1819ace9b4302000c) perf(openai): avoid repeated request copies in OAuth metadata cleanup
- [`4f8e9ae2f`](https://github.com/Wei-Shaw/sub2api/commit/4f8e9ae2f188f856266be7086d370988004a3faf) test(openai): handle metadata fixture builder return values
- [`97705d147`](https://github.com/Wei-Shaw/sub2api/commit/97705d147edef9ae673b2c0bd200b9f8b0558b7a) fix(accounts): reject wildcard model whitelist entries
- [`24d77b330`](https://github.com/Wei-Shaw/sub2api/commit/24d77b3306385e2ccb2d3336b3ea1a4aad4cb438) Add English whitelist validation hint
- [`346a7dac0`](https://github.com/Wei-Shaw/sub2api/commit/346a7dac0261416e5cb1edc05317f68796417020) Add Chinese whitelist validation hint
- [`7e75ee4b6`](https://github.com/Wei-Shaw/sub2api/commit/7e75ee4b65f56fadce41b093c106c33c6663b12e) Cover wildcard rejection and corrected model input
- [`b0f6f8391`](https://github.com/Wei-Shaw/sub2api/commit/b0f6f8391153ee4ce6753d1733ab54a1c2c81b71) fix(claude): recover requests after concurrent token rotation
- [`849aa593d`](https://github.com/Wei-Shaw/sub2api/commit/849aa593d4ac56519e8b5fb98fb7031d3cbe0d43) fix(cn-quota): refresh usage cells with account snapshots
- [`e95e9eb74`](https://github.com/Wei-Shaw/sub2api/commit/e95e9eb748b9eb2c2abd90abc8f4b7c5d73f3b11) test(cn-quota): include snapshot regressions in critical CI
- [`d850b8e3a`](https://github.com/Wei-Shaw/sub2api/commit/d850b8e3a25ce1e19d9e6c23965344000927a4bd) fix(gateway): enforce effort policies on Anthropic compatibility routes
- [`177243016`](https://github.com/Wei-Shaw/sub2api/commit/177243016569651b7dc9141a58b13d97cd5b8a88) fix(accounts): keep Gemini usage visible during refetch
- [`6d7e6b9d8`](https://github.com/Wei-Shaw/sub2api/commit/6d7e6b9d842a9a791398cf1c5f41c19e2a09dba6) fix(gateway): record native Anthropic endpoints for error diagnostics
- [`bd89d3d94`](https://github.com/Wei-Shaw/sub2api/commit/bd89d3d943c0b2dafaf927f6665ea90ccfaaada8) fix(gateway): preserve message output config with per-turn beta
- [`fc8af440e`](https://github.com/Wei-Shaw/sub2api/commit/fc8af440ed517602b682dc9d2c1630eadf6f4392) test(billing): fix peak-rate usage test clock
- [`b9d02c31a`](https://github.com/Wei-Shaw/sub2api/commit/b9d02c31a5ba47d21262ad50a37599456033e861) fix(gateway): observe Anthropic chat completion models
- [`de34f50c1`](https://github.com/Wei-Shaw/sub2api/commit/de34f50c1a2a723ae8717f56c964f1ebef3fc6ea) fix(gateway): observe Anthropic Responses models
- [`c487b215f`](https://github.com/Wei-Shaw/sub2api/commit/c487b215faea7a4158c20e74c5796d51c4393471) test(gateway): cover Anthropic compatibility observations
- [`90f115132`](https://github.com/Wei-Shaw/sub2api/commit/90f115132490b39be090459455cdb51fdda43df9) test(gateway): add observation regression cases
- [`fd6487758`](https://github.com/Wei-Shaw/sub2api/commit/fd648775810ed4348a44daaacaf38042d4927486) fix(codex): preserve additional_tools declaration IDs
- [`8dcdfff22`](https://github.com/Wei-Shaw/sub2api/commit/8dcdfff22a7ad9568a1c2f88900809dd8eeb4f56) fix(codex): normalize UUID additional tool declaration IDs
- [`734b922bd`](https://github.com/Wei-Shaw/sub2api/commit/734b922bd8e71b33ff34901c1601d49afa8dbc3d) fix(claude): default account tests to claude-sonnet-4-6
- [`8480cb0cd`](https://github.com/Wei-Shaw/sub2api/commit/8480cb0cd95710de66052dabf364f6e62d267f0f) fix(antigravity): match schema items to the selected type
- [`d085bb1a3`](https://github.com/Wei-Shaw/sub2api/commit/d085bb1a377bb42248da3bfa6d12cf205769fb56) fix(apicompat): default json_schema strict=false when converting CC to Responses
- [`c54ebd4f1`](https://github.com/Wei-Shaw/sub2api/commit/c54ebd4f124e88ece35dcc97cff9266ad4d61090) fix(kimi): parse new monthly quota plan and clear absent window snapshots
- [`694d09379`](https://github.com/Wei-Shaw/sub2api/commit/694d09379b24d93a4a57ad5dda1c0bf8d940707d) fix(kimi): harden new-plan quota parsing and absent-window cleanup
- [`a0a03be96`](https://github.com/Wei-Shaw/sub2api/commit/a0a03be96d81fd1aae4a989500cd4a2752212e80) fix(openai): normalize additional Codex context on HTTP Responses
- [`f1394a1ab`](https://github.com/Wei-Shaw/sub2api/commit/f1394a1ab4ab22e2e221e0df20a6d898074567f0) fix(web): stop caching settings-injected HTML
- [`4114fedd1`](https://github.com/Wei-Shaw/sub2api/commit/4114fedd1892ee8b14aec40c716f5685edf9663a) fix(ratelimit): lift Anthropic Fable model limit when the 7d_oi window recovers
- [`1f2242436`](https://github.com/Wei-Shaw/sub2api/commit/1f2242436293c1a64e5e17b17ca1033af902f232) fix(gateway): keep client metadata.user_id in Chat Completions to Anthropic
- [`2b86d2f6a`](https://github.com/Wei-Shaw/sub2api/commit/2b86d2f6a023bc7d1442bc2a350cb952d3616cb6) fix(gateway): stop duplicating headers on count_tokens passthrough
- [`a0340d4dd`](https://github.com/Wei-Shaw/sub2api/commit/a0340d4ddeb5831e53910dc7f93c3d3fc95bfb53) fix(websearch): honor account mode on every channel-toggle platform
- [`aee71b85b`](https://github.com/Wei-Shaw/sub2api/commit/aee71b85b9df233f914ddd6be46f51a2edd40f23) fix(gateway): restore public model in composite-routed responses
- [`7349750c0`](https://github.com/Wei-Shaw/sub2api/commit/7349750c017ca0d4efbb63cc109c6692fcc4d228) fix(gateway): limit composite model rewrite to message_start events
- [`c45ef4116`](https://github.com/Wei-Shaw/sub2api/commit/c45ef4116f0cb680a6fc87bb2e75d78869140e81) fix(pricing): check remote hash in background at startup
- [`5854a1db4`](https://github.com/Wei-Shaw/sub2api/commit/5854a1db43ad36f2c9c42e66f3dd1147a1bccff8) fix(upstream): detect dead upstream connections in about 30s
- [`94e56dd19`](https://github.com/Wei-Shaw/sub2api/commit/94e56dd192c14b300d9dc514b276164263a5b160) fix(upstream): do not fail the dial when TCP_USER_TIMEOUT cannot be set
- [`d05e8ef03`](https://github.com/Wei-Shaw/sub2api/commit/d05e8ef033db09bef1c77fe579801591322bc7cb) fix(usage): 按账户筛选时模型与端点分布的实际扣费不再替换为账号成本
- [`82cd13d1e`](https://github.com/Wei-Shaw/sub2api/commit/82cd13d1ec23420563a72a011ec94a98be5e7a63) fix(openai): fall back to memory when first-output spool creation fails
- [`38eb97bd7`](https://github.com/Wei-Shaw/sub2api/commit/38eb97bd7a2ac4635e0ba5e38a963b49554efa58) fix(openai): fail over Messages bridge errors before first content
- [`f1013f177`](https://github.com/Wei-Shaw/sub2api/commit/f1013f177f7a5918af5816f0235a61ea71699efd) fix(openai): SSE errors after keepalive, passthrough rules on exhaustion
- [`c1c9d195b`](https://github.com/Wei-Shaw/sub2api/commit/c1c9d195b3a4c7f4af5673d032c12083d4ae8cb1) fix(openai): forward upstream keepalive events before first output
- [`975b2807d`](https://github.com/Wei-Shaw/sub2api/commit/975b2807d12966823ace2428290ce53c72d59dc6) fix(openai): apply account side effects to stream errors after output on compat paths
- [`b7878328b`](https://github.com/Wei-Shaw/sub2api/commit/b7878328b699bd5f5b61e10df2db74a3723dfc7a) fix(pricing): honor explicit DeepSeek token price overrides
- [`d3716a173`](https://github.com/Wei-Shaw/sub2api/commit/d3716a1730e81ba53fd9011ae6091b670a72f6df) test(antigravity): a committed empty compat stream should release the session's sticky account
- [`5cbcb1b59`](https://github.com/Wei-Shaw/sub2api/commit/5cbcb1b59104333a74520e4da6d57de8180358da) fix(antigravity): release the sticky account after a committed empty compat stream
- [`b0d60a8c8`](https://github.com/Wei-Shaw/sub2api/commit/b0d60a8c867b54919bccdb6dca8eb86dde2644b5) fix(gemini): separate same-name parallel tool calls in streams
- [`27a61be4c`](https://github.com/Wei-Shaw/sub2api/commit/27a61be4c9e72857fcde5b0d6085d31bdb73d10b) fix(gemini): preserve final snapshots of fragmented tool calls
- [`ddb7eac63`](https://github.com/Wei-Shaw/sub2api/commit/ddb7eac6330af796c92e940f0112d34c36a24811) chore(go): upgrade to 1.27.4 to resolve security check
- [`ce026a2cb`](https://github.com/Wei-Shaw/sub2api/commit/ce026a2cbf1e1d366ddd64fe098e8a0334949a7b) fix(gemini): retain complete string argument snapshots
- [`cca745e8b`](https://github.com/Wei-Shaw/sub2api/commit/cca745e8beefa08e1afa1312e64f7f04b98bcfce) fix(scheduler): keep openai_capabilities in the scheduler projection
- [`5c96ee4d2`](https://github.com/Wei-Shaw/sub2api/commit/5c96ee4d2941b484fe1960a8d57acaa739b62507) fix(channels): persist Bedrock compatibility as a boolean
- [`32e70f3d4`](https://github.com/Wei-Shaw/sub2api/commit/32e70f3d4048ce5978dc15f72d5dd81f2e383a28) feat(gemini): 补齐 Gemini 4 Hydrogen 计费
- [`b32ad6dfe`](https://github.com/Wei-Shaw/sub2api/commit/b32ad6dfe113ed3e0ad64679d5c06adec6241961) feat(gemini): Vertex 账号支持 us/eu 多区域端点并按模型降级 global
- [`68071ddde`](https://github.com/Wei-Shaw/sub2api/commit/68071ddde1ee4de8fb31b82536568cd831fde7be) feat(gemini): Vertex 多区域端点按实际请求区域加价 10%
- [`2164ea487`](https://github.com/Wei-Shaw/sub2api/commit/2164ea48781731d3d129268d33c4220f55e6183b) fix(billing): apply Vertex multi-region multiplier to default pricing only
- [`b6c21cd5e`](https://github.com/Wei-Shaw/sub2api/commit/b6c21cd5e75acfab663a5b916572d9ed56c70d8a) fix(gemini): 原生请求按稳定前缀生成粘性会话哈希，换号按上游实际用量计费
- [`8d57f461c`](https://github.com/Wei-Shaw/sub2api/commit/8d57f461ca85f89cef0bf94ae71acc01e35042ca) fix(gemini): keep forced cache billing on sticky-session failover
- [`2101f4f94`](https://github.com/Wei-Shaw/sub2api/commit/2101f4f94005518529f0b391d823ef1cd2ade4c5) perf(billing): 同一用户的扣费按用户通道合并结算，消除余额热点行排队
- [`ade20f4e4`](https://github.com/Wei-Shaw/sub2api/commit/ade20f4e4749b743dab268e006d3ad10b58af554) perf(gateway): 请求体热点路径改用零拷贝读取，减少整段解码与复制
- [`b46b3cbab`](https://github.com/Wei-Shaw/sub2api/commit/b46b3cbabcca6781a013a38140f151786334e930) test(handler): follow normalizeCodexUntrustedContext rename from #8769
- [`9c3280098`](https://github.com/Wei-Shaw/sub2api/commit/9c3280098623989bac4d631fd8534d1c0e0ac672) perf(scheduler): 调度快照加载与数据库回源按 bucket 合并，减少大分组选号的整组复制
- [`d70a23f84`](https://github.com/Wei-Shaw/sub2api/commit/d70a23f84b7329ca76dacae590df132993d415fc) feat(gemini): add Gemini 4 Carbon support
- [`70ad8365d`](https://github.com/Wei-Shaw/sub2api/commit/70ad8365d9b5a0be5ebaf3c0065ff385f8c081fd) fix(grok): keep xAI server-side search calls out of tool calls
- [`9d5fb150b`](https://github.com/Wei-Shaw/sub2api/commit/9d5fb150b9fd8b0febed777da618b17cdf9f876c) feat(openai): optionally filter passthrough accounts by model
- [`121a4cca1`](https://github.com/Wei-Shaw/sub2api/commit/121a4cca17ae153bf675f40d14919738c0411f70) fix(scheduler): keep openai_passthrough_model_filter in the scheduler snapshot
- [`1280006a1`](https://github.com/Wei-Shaw/sub2api/commit/1280006a1f3bb7e8b29b487410b5763e06594cd1) fix(gateway): emit SSE errors after queue keepalives
- [`dd6329b98`](https://github.com/Wei-Shaw/sub2api/commit/dd6329b983eb3b3adf61159944d3c3644d14048f) fix(openai): 清洗 Python re 不支持的工具 Schema 正则
- [`e0cf4984f`](https://github.com/Wei-Shaw/sub2api/commit/e0cf4984f0d7c0d54ccb2d36292272a4231c07be) fix(openai): 删除 patternProperties 时放宽封闭对象
- [`af187f9a5`](https://github.com/Wei-Shaw/sub2api/commit/af187f9a50fcae3884875e94338088990dec00f8) fix(openai): 保留 patternProperties 删除后的 additionalProperties schema 兼容性
- [`6613ced57`](https://github.com/Wei-Shaw/sub2api/commit/6613ced57314baf6f70fe589d295b3982d6d36a0) fix(openai): let enclosing tool-schema edits supersede nested ones
- [`c8f4264f8`](https://github.com/Wei-Shaw/sub2api/commit/c8f4264f8fe8a450e1448f5d4ba07d768243ba77) fix(grok): preserve upstream media error messages and codes
- [`03a489e43`](https://github.com/Wei-Shaw/sub2api/commit/03a489e43e5103a488387e6b96602bff57a73af7) fix(grok): keep xAI flat error parsing out of the shared extractors
- [`e67746254`](https://github.com/Wei-Shaw/sub2api/commit/e677462549985a360fe032d2b66334bbbdfcad12) fix(openai): preserve quota freshness when refresh fails
- [`c4ff9b90a`](https://github.com/Wei-Shaw/sub2api/commit/c4ff9b90a917ff8b321a92819d143c969dbb0d82) test(openai): check quota snapshot timestamp type
- [`bf3951a59`](https://github.com/Wei-Shaw/sub2api/commit/bf3951a59e23974f6efecfe62e835c701e73fa25) fix(monitor): don't flip channel monitor to error on OpenAI quota refresh failure
- [`9cd1a1eb2`](https://github.com/Wei-Shaw/sub2api/commit/9cd1a1eb21e84707685e1998dcb5a3d1ceafae49) chore: update sponsors

## 合并提交（77 个）

这些提交主要记录 PR 合入及开发分支同步，不重复计为独立功能。

- [`e3151c520`](https://github.com/Wei-Shaw/sub2api/commit/e3151c5201c2456a5cac09a3582de0fc1d7913a0) Merge pull request #8767 from lishuangqiang/chore/bump-go-1.27.4
- [`a81a345a1`](https://github.com/Wei-Shaw/sub2api/commit/a81a345a17575cddc08af9efafcd92bb9fa16fa1) Merge pull request #8644 from r266-tech/codex/claude-usage-scoped-limits
- [`d7d45924e`](https://github.com/Wei-Shaw/sub2api/commit/d7d45924e8f5dc87de2a7b885932202a6b0ea307) Merge pull request #8649 from lishuangqiang/fix/alipay-stable-refund-request-id
- [`314de5d0c`](https://github.com/Wei-Shaw/sub2api/commit/314de5d0c09e73970a7cb930170f68f660245677) Merge pull request #8651 from lishuangqiang/fix/rpm-override-group-scope
- [`83a1f5dbe`](https://github.com/Wei-Shaw/sub2api/commit/83a1f5dbe82edf8b440d564811875c7460117837) Merge pull request #8653 from lishuangqiang/perf/anthropic-stream-fast-path
- [`0c641e5ed`](https://github.com/Wei-Shaw/sub2api/commit/0c641e5ed7d6894d409915f5284652aa23eb8b72) Merge pull request #8659 from lishuangqiang/fix/group-rate-sync-transaction
- [`980ddd942`](https://github.com/Wei-Shaw/sub2api/commit/980ddd942511262f49f10a51d304c1fa074d7cf7) Merge pull request #8662 from feeeei/fix/gemini-chat-response-format
- [`a6e02a37a`](https://github.com/Wei-Shaw/sub2api/commit/a6e02a37aafa07a2d149aa563120890a41ce255a) Merge pull request #8664 from jaxxjj/codex/openai-fork-prompt-cache-namespace
- [`41c567b38`](https://github.com/Wei-Shaw/sub2api/commit/41c567b38fb0a709f540f4c47477910522364eee) Merge pull request #8667 from lishuangqiang/perf/anthropic-request-json
- [`8da038854`](https://github.com/Wei-Shaw/sub2api/commit/8da0388542f01bc5bf7317730d94fe3ce2c0aedf) Merge pull request #8673 from baryon/contrib/antigravity-compat-gemini-stream-timeout
- [`0ebc8b2fd`](https://github.com/Wei-Shaw/sub2api/commit/0ebc8b2fd698739554271956df63119dfd79310f) Merge pull request #8675 from feeeei/fix/bedrock-error-classification
- [`bd3a878ef`](https://github.com/Wei-Shaw/sub2api/commit/bd3a878ef49baa32f741041755e7b9b6041ddc1e) Merge pull request #8678 from Cylion-bot/codex/disable-concurrency-ping
- [`89fce5446`](https://github.com/Wei-Shaw/sub2api/commit/89fce5446d0a449c8d0ca1dd650245747bf3cee5) Merge pull request #8682 from r266-tech/codex/preserve-sealed-call-content
- [`910d4d86b`](https://github.com/Wei-Shaw/sub2api/commit/910d4d86b2c0d389f3201415123c386c98c6be2c) Merge pull request #8684 from feeeei/fix/login-email-identity-backfill-nil
- [`fde504e34`](https://github.com/Wei-Shaw/sub2api/commit/fde504e343b2897c738c1fea7014cac7fc731371) Merge pull request #8693 from feeeei/fix/group-usage-rollup-nonblocking
- [`2df3dbcfb`](https://github.com/Wei-Shaw/sub2api/commit/2df3dbcfbb2973927dfe585bdaaa092e09fee7f7) Merge pull request #8698 from r266-tech/codex/anthropic-quota-window-start
- [`0b473ad60`](https://github.com/Wei-Shaw/sub2api/commit/0b473ad60b442828dfbecde8a9653789b862b492) Merge pull request #8703 from Anankke/fix/responses-bridge-mid-system
- [`90eb1744d`](https://github.com/Wei-Shaw/sub2api/commit/90eb1744d42adeb7c78896ab094627dcd676f7c3) Merge pull request #8723 from cyhhao/fix/keep-weekly-usage-on-5h-rollover
- [`51e76a46c`](https://github.com/Wei-Shaw/sub2api/commit/51e76a46c181ae93697f3a06702bc74ab4a45931) Merge pull request #8726 from r266-tech/codex/sticky-writeback-after-disconnect
- [`32e428389`](https://github.com/Wei-Shaw/sub2api/commit/32e4283894b45e2a7256acb374d0e4896bc6bd72) Merge pull request #8727 from r266-tech/codex/native-anthropic-response-observation
- [`a38ffc305`](https://github.com/Wei-Shaw/sub2api/commit/a38ffc30506cdb48a1228082d2e2c86e199f5ddb) Merge pull request #8728 from r266-tech/codex/chat-fallback-response-observation
- [`c6d9d48f9`](https://github.com/Wei-Shaw/sub2api/commit/c6d9d48f9a3b52d1ce7b48f020d4630162f8e486) Merge pull request #8729 from r266-tech/codex/snapshot-transient-expiry
- [`eb25511c7`](https://github.com/Wei-Shaw/sub2api/commit/eb25511c7166b3e85e911ebb969bb1c9fffba215) Merge pull request #8735 from Hato626/perf/oauth-input-metadata-cleanup
- [`472ea5752`](https://github.com/Wei-Shaw/sub2api/commit/472ea5752598d7e4581ba195aaa72989c165640b) Merge pull request #8736 from wozhenbang2004/fix/whitelist-wildcard-8710
- [`5146728d2`](https://github.com/Wei-Shaw/sub2api/commit/5146728d2f98b73330f5c316807e495f44182a7c) Merge pull request #8737 from r266-tech/codex/claude-rotated-token-retry
- [`d7ae0544e`](https://github.com/Wei-Shaw/sub2api/commit/d7ae0544e8f243b2956fbee6eaad823fbd35d03a) Merge pull request #8738 from r266-tech/codex/reactive-cn-quota-snapshots
- [`c782537ee`](https://github.com/Wei-Shaw/sub2api/commit/c782537eef5cee6d72614d963a815b4704039662) Merge pull request #8739 from r266-tech/codex/anthropic-compat-effort-policy
- [`0563176a5`](https://github.com/Wei-Shaw/sub2api/commit/0563176a5d72c51a96a525322bd13b55faf6c6df) Merge pull request #8740 from r266-tech/codex/gemini-usage-refetch
- [`e54a18654`](https://github.com/Wei-Shaw/sub2api/commit/e54a186548a0ca1aac6211a7d8c0ed6f8b939d8f) Merge pull request #8741 from r266-tech/codex/native-anthropic-error-endpoint
- [`6fd905f30`](https://github.com/Wei-Shaw/sub2api/commit/6fd905f30d890c18152c17050cba4232658a6c22) Merge pull request #8742 from r266-tech/codex/per-turn-control-beta
- [`6247d33eb`](https://github.com/Wei-Shaw/sub2api/commit/6247d33eb6009f557df3f7e1f23f3fbc74c9396e) Merge pull request #8743 from r266-tech/codex/fix-peak-rate-usage-test
- [`8a585c914`](https://github.com/Wei-Shaw/sub2api/commit/8a585c91414150bc3ce31a18809d1760422c9614) Merge pull request #8745 from wozhenbang2004/fix/anthropic-compat-observation-8714
- [`e16a349eb`](https://github.com/Wei-Shaw/sub2api/commit/e16a349ebb102caef1ec3bf5804f0c7058251636) Merge pull request #8754 from r266-tech/codex/preserve-additional-tools-id
- [`3b5310c5d`](https://github.com/Wei-Shaw/sub2api/commit/3b5310c5d54ff9fcb047e0b0bb59b19691873b0c) Merge pull request #8757 from Anandb71/arbor/sonnet-4-5-retirement
- [`202740441`](https://github.com/Wei-Shaw/sub2api/commit/202740441d6352c2218b1c2abc8479a94539d1a3) Merge pull request #8759 from r266-tech/codex/gemini-schema-array-items
- [`4763569af`](https://github.com/Wei-Shaw/sub2api/commit/4763569af25273c38476dfcd9f5dbf1965428751) Merge pull request #8761 from CooCoode/fix/8760-cc-responses-strict-default
- [`56848016c`](https://github.com/Wei-Shaw/sub2api/commit/56848016c5b57573e92cf3d5a0f9dcc2caad663e) Merge pull request #8763 from wc4440222/fix/kimi-monthly-quota
- [`d31a87a4e`](https://github.com/Wei-Shaw/sub2api/commit/d31a87a4e2024de151ac517ef7eeb16cb30085c0) Merge pull request #8769 from Undertone0809/codex/fix-codex-http-context
- [`c7df3b693`](https://github.com/Wei-Shaw/sub2api/commit/c7df3b693f075945da9ba2d3f03bb12e5a541bb3) Merge pull request #8775 from Henglie/fix/web-no-store-settings-html
- [`4cebc098c`](https://github.com/Wei-Shaw/sub2api/commit/4cebc098c32940939dc1e850f23d757cfc742d24) Merge pull request #8779 from lishuangqiang/fix/anthropic-fable-model-limit-recovery
- [`1d585c94f`](https://github.com/Wei-Shaw/sub2api/commit/1d585c94fd793e98d8e531b5eb1634405e323fec) Merge pull request #8786 from Anankke/fix/chat-completions-metadata-user-id
- [`63751ea4d`](https://github.com/Wei-Shaw/sub2api/commit/63751ea4de56e5204f2619d66e202efe441b7269) Merge pull request #8788 from Anankke/fix/count-tokens-passthrough-duplicate-headers
- [`97957edd9`](https://github.com/Wei-Shaw/sub2api/commit/97957edd9e8e7db1388fb36d5e8d1143a59fa999) Merge pull request #8790 from Anankke/fix/websearch-account-mode-platforms
- [`470c02b39`](https://github.com/Wei-Shaw/sub2api/commit/470c02b39944414f622615573bc00e0fe88275ca) Merge pull request #8796 from Anankke/fix/composite-route-public-model-response
- [`857ab9508`](https://github.com/Wei-Shaw/sub2api/commit/857ab95081dad5a47132c5e4219fd6148391bedb) Merge pull request #8798 from Anankke/fix/pricing-startup-nonblocking
- [`119b04f94`](https://github.com/Wei-Shaw/sub2api/commit/119b04f945a055607f9f078d54dc31bf3dd8f1b6) Merge pull request #8800 from Anankke/fix/upstream-tcp-keepalive
- [`568db32f2`](https://github.com/Wei-Shaw/sub2api/commit/568db32f22c1e7b315367a2f5219565e0a9b776a) Merge pull request #8806 from feeeei/fix/usage-account-filter-actual-cost
- [`9750f8e03`](https://github.com/Wei-Shaw/sub2api/commit/9750f8e039a74bdb76cead0d1b23624db3f49ddb) Merge pull request #8808 from r266-tech/codex/first-output-spool-fallback
- [`1ca302abf`](https://github.com/Wei-Shaw/sub2api/commit/1ca302abf36dd32963c9b15845be21b60720f38d) Merge pull request #8811 from Anankke/fix/messages-bridge-pre-content-failover
- [`101c2462d`](https://github.com/Wei-Shaw/sub2api/commit/101c2462ddb748e3ceb5e5a2e999fce9cf92467c) Merge pull request #8813 from qsy4/fix/openai-sse-keepalive-before-first-output
- [`992ac6330`](https://github.com/Wei-Shaw/sub2api/commit/992ac633082d33e8dd0ed519014c98018ed7258e) Merge pull request #8814 from qsy4/fix/openai-compat-stream-account-side-effects
- [`f2eaed31c`](https://github.com/Wei-Shaw/sub2api/commit/f2eaed31c365da3b986eca4b28ed8c8ca53b530b) Merge pull request #8825 from r266-tech/codex/deepseek-explicit-pricing-overrides
- [`4f3f91631`](https://github.com/Wei-Shaw/sub2api/commit/4f3f9163172b27f29988811e82cea16024d5aaeb) Merge pull request #8826 from william-drakemond/fix/compat-empty-stream-release-sticky
- [`46b879265`](https://github.com/Wei-Shaw/sub2api/commit/46b879265896850f0ee3cb4971c5bdf3dde7df2f) Merge pull request #8828 from little-greenbean/fix/gemini-parallel-tool-calls
- [`6b7cbc63b`](https://github.com/Wei-Shaw/sub2api/commit/6b7cbc63bed7defa0a5b83c21f5a315a1ab02beb) Merge pull request #8837 from Daiyangcheng/fix/seedance-scheduler-capability-projection
- [`d70676756`](https://github.com/Wei-Shaw/sub2api/commit/d70676756fd3a35b854d41aa46c07935bbef283f) Merge pull request #8840 from r266-tech/codex/channel-bedrock-compat-toggle
- [`1cf7eb48c`](https://github.com/Wei-Shaw/sub2api/commit/1cf7eb48c46f5f1fbbe75db7fe955a4f8a699448) Merge pull request #8746 from feeeei/feat/gemini-4-hydrogen-pricing
- [`2286c26d1`](https://github.com/Wei-Shaw/sub2api/commit/2286c26d1b3e1913ad55b05ade9bc9ef6b121fdb) Merge remote-tracking branch 'gh/main' into fix-8827
- [`f242a6ea2`](https://github.com/Wei-Shaw/sub2api/commit/f242a6ea2327e2c849b8765bcfd3bf9e2abc7160) Merge pull request #8827 from feeeei/feat/gemini-vertex-multi-region
- [`e391d5fa3`](https://github.com/Wei-Shaw/sub2api/commit/e391d5fa386e6e59b4b0f93e56cd9e1778393f04) Merge remote-tracking branch 'gh/main' into fix-8822
- [`04503e001`](https://github.com/Wei-Shaw/sub2api/commit/04503e001e3d5ac3c156b8293accbebc31afeb17) Merge pull request #8822 from feeeei/fix/gemini-native-sticky-session-hash
- [`fb07aaee2`](https://github.com/Wei-Shaw/sub2api/commit/fb07aaee23e9c5b2a6649518f65effb57b6b0299) Merge remote-tracking branch 'gh/main' into fix-8816
- [`b140d5db8`](https://github.com/Wei-Shaw/sub2api/commit/b140d5db8cfbfa651a388960a24098fe99ad0a53) Merge pull request #8816 from feeeei/perf/usage-billing-lane-zero-copy
- [`7f3fe5bfd`](https://github.com/Wei-Shaw/sub2api/commit/7f3fe5bfde146f940aaad88e89f6e8944e1f5ceb) Merge main into perf/scheduler-snapshot-coalesce
- [`8ed701382`](https://github.com/Wei-Shaw/sub2api/commit/8ed701382cbc4eadecc0de0684698ad85cf7cce0) Merge pull request #8833 from feeeei/perf/scheduler-snapshot-coalesce
- [`03c17491b`](https://github.com/Wei-Shaw/sub2api/commit/03c17491b3ab69ff36ae7d45e15d3d6088be0c2d) Merge pull request #8661 from xy3xy3/feat/gemini-4-carbon-8658
- [`4328510f2`](https://github.com/Wei-Shaw/sub2api/commit/4328510f285c2fbcd57fe4e57ab3a73a86e62368) Merge pull request #8834 from Anankke/fix/grok-server-search-calls
- [`42efee245`](https://github.com/Wei-Shaw/sub2api/commit/42efee245ea1108f51b94859d9bbbc372182b058) Merge remote-tracking branch 'gh/main' into fix-8748
- [`7a19bbef7`](https://github.com/Wei-Shaw/sub2api/commit/7a19bbef73b76e9158944d4a94c7e58bb5c78912) Merge pull request #8748 from r266-tech/codex/passthrough-model-filter
- [`f8d891d6f`](https://github.com/Wei-Shaw/sub2api/commit/f8d891d6fbdfb3c378f827d897f45b80438d974f) Merge pull request #8724 from r266-tech/codex/anthropic-errors-after-keepalive
- [`a2dfcc259`](https://github.com/Wei-Shaw/sub2api/commit/a2dfcc259f7d452812b495b0d3aff56f757057f9) Merge remote-tracking branch 'gh/main' into fix-8772
- [`3e8e3254d`](https://github.com/Wei-Shaw/sub2api/commit/3e8e3254d1a5c9f4e2bf66d05ca95ba57bdf6297) Merge pull request #8772 from Henglie/fix/openai-tool-schema-python-regex
- [`589166e32`](https://github.com/Wei-Shaw/sub2api/commit/589166e32012d23b37e3f667c36d355c0512f500) Merge remote-tracking branch 'gh/main' into fix-8669
- [`7e4c614bc`](https://github.com/Wei-Shaw/sub2api/commit/7e4c614bcfb7fc93c955f6da3227c4c0cdfab8cb) Merge pull request #8669 from r266-tech/codex/grok-top-level-errors
- [`326ae1ffc`](https://github.com/Wei-Shaw/sub2api/commit/326ae1ffc9a1d7d72ed99125b19f0da4686ad9c5) Merge commit 'd70676756fd3a35b854d41aa46c07935bbef283f' into fix/openai-quota-freshness
- [`a535a921f`](https://github.com/Wei-Shaw/sub2api/commit/a535a921f189547f0512ea68fc564ad05f45896f) Merge remote-tracking branch 'gh/main' into fix-8747
- [`ef2920300`](https://github.com/Wei-Shaw/sub2api/commit/ef29203002bf134560b78627e675a79547b88ab3) Merge pull request #8747 from ruixingshi/fix/openai-quota-freshness
