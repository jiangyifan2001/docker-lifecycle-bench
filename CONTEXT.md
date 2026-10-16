# Docker Lifecycle Bench

本项目定义并实现一套可复现的 benchmark suite，用来测量不同 host platform 上 Docker container lifecycle 的 latency、throughput 与 saturation behavior。

## Language

**Benchmark Suite**:
包含可执行 workload、测量工具、环境验收、原始结果和分析流程的完整仓库。
_Avoid_: CPU test, test plan

**System Microbenchmark**:
使用最小 workload 聚焦 Host Platform 中一条明确系统路径的 benchmark；本项目聚焦 Docker Lifecycle，但仍会跨越多个系统组件。
_Avoid_: CPU microbenchmark, Application benchmark

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

**Core Profile**:
使用最小镜像和最少外围功能来表征 Docker Lifecycle 控制路径的正式 workload；它不代表完整的 agent sandbox workload。
_Avoid_: Sandbox-realistic profile, Production profile

**Valid Result**:
在受支持的 Reference Environment 中完整通过正确性与完整性验收、可以保存和分析的一次 benchmark 结果。
_Avoid_: Comparable Result

**Comparable Result**:
来自同一 comparison campaign、满足共同软件栈与配置契约、可以进行横向排名的 Valid Result。
_Avoid_: Any Valid Result

**Diagnostic Result**:
为了归因而改变观测工具、cache 或系统配置后产生的结果；它只能解释差异，不能进入 headline 排名。
_Avoid_: Comparable Result

**Lifecycle Run**:
在 daemon 已运行且镜像已预加载的条件下测量 Docker Lifecycle 的 run；结果不包含 daemon startup 或 image provisioning。
_Avoid_: Cold Start Run

**Daemon-cold Run**:
单独表征 Docker daemon 重启后初始 lifecycle 行为的 run。
_Avoid_: Lifecycle Run, Cold Start Run

**Image Provisioning Run**:
单独表征 image transfer、解压、导入和 snapshot preparation 的 run。
_Avoid_: Lifecycle Run, Cold Start Run

**Forced Teardown**:
通过不可协商的终止信号结束容器进程，然后等待退出并请求移除容器对象的 teardown profile。
_Avoid_: Graceful Teardown

**Graceful Teardown**:
允许容器进程接收终止信号并执行应用退出逻辑，然后请求移除容器对象的 teardown profile。
_Avoid_: Forced Teardown

**Readiness Point**:
Core probe 执行到预先定义并可观测位置的时刻；它既不是处理器执行的首条用户态指令，也不是通用应用就绪。
_Avoid_: First user instruction, Application ready

**Instrumented Run**:
包含 Readiness Point 通知、因而能够计算 start-to-ready latency 的 run。
_Avoid_: Uninstrumented Run

**Uninstrumented Run**:
不携带 Readiness Point 通知路径、用于最小 full-cycle latency 和 throughput headline 的 run。
_Avoid_: Instrumented Run

**Remove Acknowledgement**:
Docker remove API 成功返回、容器对象逻辑删除请求完成的用户可观察边界；它不表示所有底层存储已经物理回收。
_Avoid_: Physical removal, Storage Drain

**Storage Drain**:
测量窗口结束后，延迟的 snapshot cleanup、garbage collection 与资源释放达到验收条件的状态。
_Avoid_: Remove Acknowledgement

**Comparison Campaign**:
使用同一冻结比较契约、在一组 Host Platforms 上执行并判断 Comparable Results 的实验集合。
_Avoid_: Unrelated benchmark runs

**Host Harness**:
在被测 host 上生成 load、执行 Docker Lifecycle、采集时间戳并写入原始结果的受测量约束程序。
_Avoid_: Coordinator, Analysis tool

**Harness Client**:
创建或提交 Campaign Spec、调用 Host Harness 并读取 Result Bundle 的用户入口；它不执行正式计时或负载调度。
_Avoid_: Host Harness, Timing engine

**Coordinator**:
在计时路径之外选择 host、触发 run 和收集 Result Bundles 的编排角色。
_Avoid_: Host Harness

**Campaign Spec**:
经过 schema 验证、默认值展开和矩阵解析后，完整描述一次 Comparison Campaign 的不可变规范。
_Avoid_: User YAML, Runtime flags

**Result Bundle**:
一个 run 的自包含、可校验结果目录，保存 Campaign Spec、环境 manifest、原始记录、验收结论和派生摘要。
_Avoid_: Log directory, Summary report

**Derived Artifact**:
由 Result Bundle 中的原始事实可重复生成的表格、列式数据或图表；它不能替代或修改原始记录。
_Avoid_: Raw result

**Benchmark Operator**:
预先获准访问本机 Docker socket、负责启动 benchmark 的可信本地用户；它可以是现有用户或专用账户。
_Avoid_: Automatically created user, Container user

**Reference Stack**:
v1 用来产生 Comparable Results 的冻结 Docker、runtime、storage、cgroup 与 security profile。
_Avoid_: Any Docker installation

**Release Manifest**:
把 source revision、toolchains、build flags、OCI index、各平台 artifacts 和 schemas 绑定到一个 benchmark release 的校验清单。
_Avoid_: Environment manifest, Result Bundle

**Run Status**:
描述一次 run 是成功、被环境阻塞、执行失败、结果无效还是被中断的机器可解析结论。
_Avoid_: Comparable Result, Process exit code

**Local-NUMA Baseline**:
将 Host Harness、Docker control plane 与 benchmark containers 放在同一目标 NUMA node，并让关键内存分配保持本地的 headline placement profile。
_Avoid_: Whole-system profile, Remote-client NUMA Sensitivity

**Remote-client NUMA Sensitivity**:
把 Host Harness 放在目标 NUMA node 之外，同时保持 Docker control plane 与 benchmark containers 位于目标 node 的 sensitivity profile。
_Avoid_: Local-NUMA Baseline, Cross-platform headline

**NUMA Penalty**:
同一 Host Platform 和 benchmark 配置下，Remote-client NUMA Sensitivity 相对 Local-NUMA Baseline 的结果变化。
_Avoid_: Processor ranking

## Phase 0 implementation

首个可执行里程碑是在 Hygon 7490H bare-metal host 上完成一次 Diagnostic
Smoke Run。它只实现 `FullLifecycle`，不产生跨平台 Comparable Result。

- Go binary `dlbench` 是唯一 Host Harness、计时引擎和负载调度器。
- Python package `dlbench_client` 是薄 wrapper，通过 subprocess 调用
  `dlbench`；它不实现正式计时或调度。
- 静态 C/musl probe 作为 container PID 1，写出一个 binary READY frame 后
  阻塞等待 teardown。
- Phase 0 使用 `CreateRemove`、`ReusedStartExit` 和 `FullLifecycle` 名称，避免
  将旧称 L1/L2/L3 与处理器 cache levels 混淆。
- Phase 0 结果固定分类为 Diagnostic，并明确省略不受样本量支持的 p99.9。
