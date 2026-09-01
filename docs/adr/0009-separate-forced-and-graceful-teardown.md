---
status: accepted
---

# Separate forced and graceful teardown

v1 的 Core Profile 同时包含 Forced Teardown 与 Graceful Teardown，并作为不同场景分别测量；Forced 是 headline，Graceful 使用行为固定的最小 probe 并作为独立结果。两类结果不合并，且必须明确说明 Docker remove acknowledgement 与底层存储物理回收不是同一个完成边界。
