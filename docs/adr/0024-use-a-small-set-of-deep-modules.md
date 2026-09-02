---
status: accepted
---

# Use a small set of deep modules

v1 以薄 `dlbench` CLI 调用少量 deep modules：campaign 编排、measurement、qualification 和 append-only bundle；Docker transport、clock 与 test fakes 是 internal seams。create/start/exit/remove 只是 measurement module 产生的阶段记录，不成为独立 public modules，也不建立 v1 plugin framework。
