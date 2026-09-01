---
status: accepted
---

# Lock a multi-platform index and native artifacts

x86-64 与 Arm64 共享 source commit、OCI image index digest、toolchain policy 和 build flags，同时分别锁定 image manifest、harness 和 probe digest。正式结果只允许在匹配 ISA 的原生 bare metal 上产生，禁止 QEMU、binfmt 或其他处理器模拟执行 benchmark artifacts。
