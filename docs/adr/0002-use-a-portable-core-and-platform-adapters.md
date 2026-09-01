---
status: accepted
---

# Use a portable core and platform adapters

核心 benchmark 规范不限定处理器 vendor 或 ISA，平台差异通过 capability detection 和 platform adapters 表达；首批正式支持 x86-64 与 Arm64，并允许将来扩展其他 Linux/Docker processor platforms。这样避免把 AMD、Hygon 或 amd64 假设伪装成通用要求，同时也不声称未经验收的平台已经受支持。
