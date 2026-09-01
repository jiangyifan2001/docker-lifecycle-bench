---
status: accepted
---

# Use Python for offline analysis outside the timed path

Python tooling 使用冻结依赖对不可变 Result Bundles 做离线统计、Parquet 转换和绘图，不进入正式计时窗口；Go tooling 负责在线写入、基本验收与轻量摘要。分析工具只能新增 Derived Artifacts，不能修改原始结果。
