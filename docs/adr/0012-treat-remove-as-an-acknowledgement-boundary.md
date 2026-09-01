---
status: accepted
---

# Treat remove as an acknowledgement boundary

Docker remove 成功返回只定义 Remove Acknowledgement，因此通用指标使用 `remove_ack_latency`、`destroy_ack_latency` 和 `cycle_ack_latency`。headline throughput 仍按成功 acknowledgement 计数，但结果必须同时通过 post-window Storage Drain、snapshot 残留与存储占用验收；物理回收 latency 只能作为 backend-specific diagnostic metric。
