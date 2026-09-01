---
status: accepted
---

# Separate forced and graceful teardown

Forced Teardown 与 Graceful Teardown 作为不同场景分别测量，避免把 Docker/runtime 清理成本与应用信号处理成本混为一谈。两类结果不合并，且必须明确说明 Docker remove acknowledgement 与底层存储物理回收不是同一个完成边界。
