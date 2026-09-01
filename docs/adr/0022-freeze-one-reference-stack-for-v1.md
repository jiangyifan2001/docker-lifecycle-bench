---
status: accepted
---

# Freeze one Reference Stack for v1

v1 headline Reference Stack 使用 rootful Docker、cgroup v2/systemd、containerd image store、overlayfs snapshotter、默认 seccomp、启用的 AppArmor 和 kernel mitigations，并禁止 rootless、user namespace remapping 与 custom runtime。classic overlay2 或不同 security/storage 配置只能进入独立 cohort 或 Diagnostic Results，不能与 Reference Stack 结果直接比较。
