---
status: accepted
---

# Let Python invoke the Go binary as a subprocess

Python Harness Client 通过 subprocess 调用 Release Manifest 中锁定的 `dlbench` binary，并只交换 versioned Campaign Specs、控制状态与 Result Bundle references。v1 不使用 Go shared-library FFI，也不启动常驻 benchmark daemon，从而避免第二套 ABI、内存生命周期或网络 interface。
