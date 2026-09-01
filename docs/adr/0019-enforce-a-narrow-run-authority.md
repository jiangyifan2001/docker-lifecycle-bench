---
status: accepted
---

# Enforce a narrow run authority

环境检查保持只读，正式 run 只能管理带本次 run identity 的 benchmark containers 并写入一个全新的 Result Bundle。Benchmark Operator 可以是已有可信用户或专用 `dockerbench`，但必须预先获得 Docker socket 权限；工具不得创建用户、修改 group、自动提权、安装软件、修改固件或启动配置、重启 daemon、格式化存储、覆盖已有结果或清理 Docker 全局状态。需要扩大权限时必须返回 `BLOCKED`。
