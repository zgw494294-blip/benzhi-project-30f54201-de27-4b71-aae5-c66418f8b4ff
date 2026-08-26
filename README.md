# 科研温室隔离舱启用验证工作台

本项目用于在隔离舱投入实验前，沿一条可审计闭环完成边界登记、方案冻结、压差/气密/门禁互锁/净化恢复测试、偏差定向复测、生物安全复核，以及启用凭据签发和真实性校验。

服务由 Go 标准库实现，直接提供原生 HTML、CSS、JavaScript 单页工作台和同源 JSON API，不需要 Node 构建链。验证案采用 `expectedVersion` 乐观并发控制；所有写请求使用 `Idempotency-Key` 去重。数据保存在带 `schemaVersion` 的 JSON 快照和哈希链审计日志中，快照通过临时文件、`Sync` 与原子 `Rename` 提交。

## 构建与运行

```text
go build ./...
go run ./cmd/server
```

默认仅监听 `127.0.0.1:19081`，浏览器访问 `http://127.0.0.1:19081`。可显式指定其他回环端口：

```text
go run ./cmd/server -addr=127.0.0.1:19120
```

也可设置 `PORT` 为纯端口号，服务将绑定 `127.0.0.1:<PORT>`。为避免意外暴露案卷，程序拒绝 `0.0.0.0` 等非回环监听地址。正常模式默认将快照和审计日志写入 `data/`，可用 `-data` 指定目录。

## 测试与自检

运行全部单元测试、持久化恢复测试与 HTTP 测试：

```text
go test ./...
```

运行有界全流程自检：

```text
go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
```

自检会启动真实回环 HTTP 监听器，通过公开 API 完成建案、冻结方案、四项合格测试、提交复核、批准签发和凭据摘要复算，然后关闭服务并自行退出；自检数据使用临时目录，不写入正常案卷目录。

## 主要流程

1. 创建验证案；方案冻结前可按当前版本受控修订边界与验收限值，每次修订保存逐字段差异。
2. 冻结前生成四类检查点预检报告和摘要，用户必须使用绑定案卷版本、规则版本及检查点顺序的一次性确认标识冻结。
3. 按顺序单项或原子批量录入时间、仪器校准证书、有效期、适用类型、见证人和原始测量，服务按冻结规则自动判定并生成证据摘要。
4. 失败时自动计算建议风险等级并锁定定向复测范围；处置需登记分级理由、责任人、期限、根因和纠正措施，详情会投影临期或逾期状态。
5. 全部最新结果合格且偏差关闭后生成结构化复核清单。复核员逐项确认后方可批准；退回问题必须关联清单项和受影响检查点，再次送审需说明处理结果。
6. 使用凭据编号重新加载封存快照并复算摘要，确认凭据真实性。

浏览器工作台会展示检查点进度、逐条判定规则、失败原因、偏差范围、送审阻断原因和完整状态时间线。

## API 概览

- `GET /`：浏览器工作台。
- `GET /healthz`：健康检查。
- `GET /api/system`：规则版本、规则词典与本地持久化诊断。
- `GET /api/cases?q=`、`POST /api/cases`：检索和创建验证案。
- `GET /api/cases/{caseId}`：案卷、就绪投影、偏差时限、证据统计、时间线和审计事件。
- `PUT|PATCH /api/cases/{caseId}`、`POST /api/cases/{caseId}/revise`：按 `expectedVersion` 修订草拟案卷完整基础资料。
- `POST /api/cases/{caseId}/freeze/preflight`：生成冻结预检报告、预览摘要和一次性确认标识。
- `POST /api/cases/{caseId}/freeze`：携带 `confirmationToken` 冻结与预览一致的测试方案。
- `POST /api/cases/{caseId}/runs`：录入并判定单项测试，或在 `runs` 中原子提交连续检查点批次。
- `POST /api/cases/{caseId}/deviations/{deviationId}/remediate`：记录整改并生成定向复测。
- `POST /api/cases/{caseId}/submit-review`：提交安全复核。
- `POST /api/cases/{caseId}/review`：带理由退回或批准签发。
- `GET /api/credentials/{credentialId}/verify`：复算并校验凭据摘要。

写接口要求 `Content-Type: application/json` 和非空 `Idempotency-Key`。除建案外，命令体必须携带从最新案卷读取的 `expectedVersion`；方案执行还必须携带当前 `protocolRevision`。版本冲突返回 HTTP `409`，并在适用时给出 `currentVersion` 与 `conflictFields`；状态机阻止的操作返回 HTTP `422`。
