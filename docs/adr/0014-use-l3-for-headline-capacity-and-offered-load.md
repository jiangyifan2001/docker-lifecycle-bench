---
status: accepted
---

# Use L3 for headline capacity and offered load

串行 latency 同时执行 L1 create/remove、L2 reused-object start/exit 和 L3 full lifecycle，以便分阶段定位；Capacity 与 Offered-load headline 只使用 L3。L1/L2 的并发结果只能作为 Diagnostic Results，避免产生多个含义不同的“最大吞吐”。
