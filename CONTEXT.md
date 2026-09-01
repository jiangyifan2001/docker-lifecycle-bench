# Docker Lifecycle Benchmark

本项目定义并实现一套可复现的 benchmark suite，用来测量不同 host platform 上 Docker container lifecycle 的 latency、throughput 与 saturation behavior。

## Language

**Benchmark Suite**:
包含可执行 workload、测量工具、环境验收、原始结果和分析流程的完整仓库。
_Avoid_: CPU test, test plan

**Docker Lifecycle**:
一个 Docker container 从对象创建、进程启动与就绪、进程退出到容器对象移除的完整生命过程。
_Avoid_: Sandbox lifecycle, cold start

**Lifecycle Cycle**:
一次完整且成功的 Docker Lifecycle；只有到达规定的最终移除边界才计为 completed cycle。
_Avoid_: Request, operation

**Host Platform**:
共同决定 lifecycle 结果的整机环境，包括处理器、内存与 NUMA、存储、固件、OS、kernel 和 container software stack。
_Avoid_: CPU, CPU platform

**Processor Platform**:
Host Platform 中由处理器 ISA、vendor、model 与 topology 构成的部分；它不是 benchmark 结果的唯一归因对象。
_Avoid_: AMD platform, Hygon platform

**Capacity Run**:
使用 closed-loop workers 寻找 Host Platform 可持续完成 Lifecycle Cycle 的容量、饱和点和饱和后的错误边界。
_Avoid_: Maximum CPU throughput

**Offered-load Run**:
按预定 arrival rate 提交 lifecycle 请求，用来测量指定负载下的排队、latency、成功率与 SLO 行为。
_Avoid_: Throughput run

**Reference Environment**:
当前版本允许产生可比较正式结果的独占 bare-metal Linux 环境；虚拟化和云环境不属于当前结果域。
_Avoid_: Any Linux host
