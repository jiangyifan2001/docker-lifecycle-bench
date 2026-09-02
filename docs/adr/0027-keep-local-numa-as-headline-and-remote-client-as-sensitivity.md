---
status: accepted
---

# Keep Local-NUMA as headline and Remote-client as sensitivity

v1 以 Local-NUMA Baseline 产生 fixed-resource headline，同时运行 Remote-client NUMA Sensitivity 来量化 NUMA Penalty；后者不进入跨平台主排名。该实验定义对处理器厂商与 ISA 中立，并要求在同一 host 上保持 Docker control plane、containers、软件栈和 workload 不变，只改变 Host Harness 相对目标 node 的位置。
