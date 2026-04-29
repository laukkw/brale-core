# Prompt Registry 运维说明

本文档描述 Brale-Core 当前 `prompt_registry` 的最小运维面。

## 当前行为

- 启动时会把代码内置 prompt defaults seed 到 `prompt_registry`
- 运行时读取顺序是：数据库中的 active prompt -> 代码内置默认值
- prompt loader 带进程内缓存；更新数据库后需重启进程方可生效（进程内缓存不支持热重载）

## 查询

使用 `bralectl llm prompts list`：

```bash
bralectl llm prompts list --db 'postgres://brale:brale@localhost:5432/brale?sslmode=disable'
bralectl llm prompts list --role agent --active-only
```

字段说明：

- `role`: `agent` / `provider` 等逻辑角色
- `stage`: 例如 `indicator` / `structure` / `mechanics`
- `version`: prompt 版本号；内置默认值会显示为 `builtin`
- `active`: 是否为当前优先使用版本

## 更新流程

推荐流程：

1. 插入新版本 prompt
2. 将目标版本标记为 `active=true`
3. 将旧版本标记为 `active=false`
4. 重启 `brale-core`
5. 用 `bralectl llm rounds` 或 `/api/llm/rounds` 核对 `prompt_version`

示例 SQL：

```sql
INSERT INTO prompt_registry (role, stage, version, system_prompt, description, active)
VALUES ('agent', 'indicator', 'v2026-04-15', '...', 'indicator prompt rollout', true);

UPDATE prompt_registry
SET active = false
WHERE role = 'agent' AND stage = 'indicator' AND version = 'builtin';
```

## 回滚

如果新 prompt 表现异常：

1. 将旧版本重新设为 `active=true`
2. 将问题版本设为 `active=false`
3. 重启进程
4. 用 `bralectl llm eval` 和 `bralectl llm rounds` 观察最近 round 的 `prompt_version`、`error`、`latency`

## 当前限制

- 现在的 `llm eval` 是基于 `llm_rounds` 元数据的离线评估，不会重放模型调用
- 还没有专门的 prompt 发布 API；当前以 SQL + CLI 查询为主
- 还没有在线 cache invalidate 接口；变更后建议重启
