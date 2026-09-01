---
status: accepted
---

# Use Go for the harness and static C for the probe

v1 使用 Go 和官方 Docker Engine client 实现 host harness，以获得跨 x86-64/Arm64 的可移植并发控制与 API 支持；Core probe 使用静态 C/musl，以减少语言 runtime 初始化对最小启动路径的影响。harness 与 probe 都使用显式的 `CLOCK_MONOTONIC_RAW` 封装，Go runtime、GC 和 driver saturation 必须作为测量有效性风险受到监控。
