---
status: accepted
---

# Separate lifecycle, daemon-cold, and image-provisioning results

Lifecycle、Daemon-cold 和 Image Provisioning 分别执行和计算，不使用含义不确定的单一“冷启动”数字。正式 lifecycle headline 使用已运行的 daemon 和预加载镜像，另外两个 suite 各自报告启动或镜像准备成本。
