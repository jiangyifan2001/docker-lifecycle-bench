---
status: accepted
---

# Ship an Ubuntu adapter with a distribution-neutral core

核心 benchmark 规范保持 Linux distribution-neutral，v1 提供 Ubuntu Server 24.04 LTS adapter，并在 x86-64 与 Arm64 上分别构建和验收。两个 ISA 共享发行版 release 和 benchmark source，但各自记录 installation image、kernel package、artifact 与 image manifest digest，而不要求字节相同的跨 ISA 二进制。
