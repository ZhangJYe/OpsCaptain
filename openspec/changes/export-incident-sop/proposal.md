## Why

事故诊断已经沉淀了策略、过程、证据与结论，但这些信息不能被运维人员直接带入团队 SOP 或复盘流程。需要让用户从已完成的事故记录中导出一份带明确时间与人工复核边界的 SOP 草稿。

## What Changes

- 在事故记录的诊断结论区域提供“导出 SOP”操作。
- 将当前事故的元数据、实际策略、根因、证据、处置建议和轮次整理为 Markdown 文件。
- 在文件中记录事故创建、更新时间、完成时间（如有）和导出时间，并标注它是待人工复核的 SOP 草稿。
- 未选择事故、尚无可导出内容或导出失败时提供明确提示，不创建或修改服务端数据。

## Capabilities

### New Capabilities

- `incident-sop-export`: 从已创建的事故记录生成并下载可人工复核的 Markdown SOP 草稿。

### Modified Capabilities

- 无。

## Impact

- 前端：`frontend/src/components/incident/IncidentView.tsx` 及其事故数据展示。
- 浏览器：使用本地下载生成 Markdown 文件；不新增后端 API、持久化或外部依赖。
- 测试：覆盖 SOP 内容、日期格式、文件命名与不可导出状态，并执行前端构建验证。
