---
status: accepted
---

# Preserve results across failure and interruption

run 使用 `SUCCESS`、`INVALID_SPEC`、`BLOCKED`、`RUN_FAILED`、`INVALID_RESULT` 和 `INTERRUPTED` 等不同 Run Status；只要执行已经开始，即使失败也保留 Result Bundle 和原始错误。第一次中断停止新 cycle、限时 drain 并仅清理本次 run 的对象；再次中断可以立即退出，但不得扩大清理范围或执行全局 prune。
