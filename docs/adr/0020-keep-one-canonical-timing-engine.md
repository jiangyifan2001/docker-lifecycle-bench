---
status: accepted
---

# Keep one canonical timing engine

v1 只有一个 canonical Go Host Harness 负责计时、load scheduling、Docker transport 和原始结果写入；Python 提供 Harness Client 和离线分析，而不是第二套 timing engine。Go CLI 与 Python client 都通过同一 Campaign Spec 和 Result Bundle interface 使用该实现，因此入口语言不同不会改变 Comparable Result 的测量语义。
