---
status: accepted
---

# Measure Docker lifecycle performance on host platforms

本项目测量 Host Platform 上的 Docker Lifecycle performance，而不把结果直接命名或解释为 CPU performance。Docker lifecycle 同时经过处理器、kernel、存储、网络以及 Docker/containerd/runc；只有额外的受控诊断证据才能把差异归因到 Processor Platform。
