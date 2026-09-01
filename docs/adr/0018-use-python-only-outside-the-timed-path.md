---
status: accepted
---

# Use Python for offline analysis outside the timed path

Python tooling 可以作为 Harness Client 创建 Campaign Spec、调用 canonical Go Host Harness，也可以使用冻结依赖对不可变 Result Bundles 做离线统计、Parquet 转换和绘图；它不实现正式计时或负载调度。Go tooling 负责 timed path、在线写入、基本验收与轻量摘要；分析工具只能新增 Derived Artifacts，不能修改原始结果。
