---
status: accepted
---

# Expose one versioned CLI interface

canonical Go binary 命名为 `dlbench`，对外提供 `plan`、`doctor`、`run`、`validate`、`summarize` 和 `compare` commands；Python Harness Client 调用同一 interface，不复制执行逻辑。commands 使用 versioned Campaign Spec、Result Bundle 和稳定 exit status 作为 interface，内部 module 与 Docker client 细节不对调用者公开。
