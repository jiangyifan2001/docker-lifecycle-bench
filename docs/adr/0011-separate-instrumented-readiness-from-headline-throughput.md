---
status: accepted
---

# Separate instrumented readiness from headline throughput

Instrumented Runs 在 start 前 attach probe stdout，并通过带版本和 run nonce 的固定长度 frame 传递 Readiness Point 的 raw timestamp；Uninstrumented Runs 不包含该通知路径并承担 full-cycle throughput headline。v1 要求 harness 与 probe 位于同一 time namespace，否则拒绝产生结果，而不尝试 offset 校准。
