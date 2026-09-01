---
status: accepted
---

# Build release artifacts off the benchmark host

Go harness、静态 C probe 和 OCI images 在冻结的专用 build environment 中生成，由 Release Manifest 记录 source revision、toolchains、flags 和各平台 digest。benchmark host 不安装编译工具链，只校验 checksum、ELF/ISA 并执行 native smoke test；正式 campaign 期间禁止重新 build。
