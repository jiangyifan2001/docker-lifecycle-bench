---
status: accepted
---

# Keep the benchmark in one repository

v1 将 host harness、Core probe、Ubuntu adapter、versioned schemas、analysis tooling 和文档保存在一个 repository，并通过同一个 release manifest 冻结版本。当前不拆分 repositories，也不建立 runtime plugin interface，以免增加 artifact 版本错配和浅层 indirection。
