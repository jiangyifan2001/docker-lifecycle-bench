---
status: accepted
---

# Normalize campaign specs and preserve result bundles

用户以严格、带 `schema_version` 的 YAML 描述 campaign，未知字段会失败；执行前展开所有默认值和矩阵并生成 canonical JSON Campaign Spec。每个 run 写入不可覆盖的 Result Bundle，其中 NDJSON 和 manifest JSON 是原始事实，JSON/CSV/Parquet 与图表都是可重新生成的 Derived Artifacts。
