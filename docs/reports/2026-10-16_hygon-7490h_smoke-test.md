# docker-lifecycle-bench：Hygon 7490H smoke test 报告

报告日期：2026-10-16。测试主机：liuhui。时间如无特别说明，均为北京时间（UTC+8）。

本报告分析已经保存的测试记录与对应实现，没有重新运行 benchmark，也没有连接或试跑 AMD 机器。原始结果包未作修改。

## 1. 结论

这次 smoke 已达成当前目标：在 liuhui 上跑通“创建容器 → 启动并收到 probe READY → 强制终止 → 等待退出 → 删除容器”的完整链路，并保存可复核的数据。

- 小规模预检（canary）完成 **46 次**，正式 smoke 完成 **2,909 次**，记录中的失败数均为 **0**。
- 正式 smoke 的 1,000 次串行测量中，完整生命周期 p50 为 **196.53 ms**、p99 为 **247.47 ms**；启动到 probe READY 的 p50 为 **107.54 ms**。
- Capacity 测量从 1 个 worker 的 **5.28 次/秒**上升到 8 个 worker 的 **26.22 次/秒**。单位是“完成的完整生命周期/秒”，不是单独的启动次数/秒。
- 结果属于 **Phase 0 / diagnostic**：证明功能链路与数据记录可用，不代表 Hygon 的性能上限，也不能据此做处理器平台排名。
- **代码适用于满足环境条件的 AMD x86-64 Linux 裸机。** 当前没有 Hygon 厂商锁定；但本报告没有验证任何具体 AMD 主机，因此结论是兼容性判断，不是“AMD 已测试通过”。

## 2. 测试环境与版本

以下主要来自正式结果包的 [environment.json][environment]、[manifest.json][manifest] 和 [campaign.normalized.json][campaign]。

| 项目 | 本次实际记录 |
|---|---|
| 主机及处理器标识 | liuhui；用户确认是 Hygon 7490H；系统原样报告 `Hygon C86-4G/H (OPN:7446)`，两种标识保留，不擅自等同或改写 |
| CPU 拓扑 | 2 个 socket，每 socket 96 核；192 个在线逻辑 CPU，编号 0–191；另有 192 个离线编号；每核在线线程数为 1 |
| NUMA | 12 个节点，各包含 16 个在线 CPU；记录了节点内存容量及距离矩阵 |
| 内存 | Docker 报告 1,622,921,183,232 bytes，约 1.48 TiB |
| 操作系统 / 内核 | Ubuntu 22.04.5 LTS / Linux 5.15.0-119-generic，x86_64 |
| 裸机检查 | `systemd-detect-virt` 报告裸机；环境检查 `eligible=true` |
| Docker | 29.1.3，rootful；cgroup v2，systemd cgroup driver |
| 存储与安全配置 | `overlayfs`；Docker 数据目录 `/var/lib/docker`；AppArmor、内置 seccomp、cgroup namespace |
| 频率信息 | 系统报告 boost disabled，频率范围 1.4–2.2 GHz；这不是测试期间实际频率的连续测量 |
| 工具 | `dlbench 0.1.0-smoke`；Go 1.27.1；linux/amd64 |
| 测试源码版本 | `25bd3fe9a7f98af8174aff828b61481a001daeab` |
| probe 镜像引用 | `dlbench-probe:smoke` |
| 容器配置 | `networkMode=none`；日志 driver 为 `none`，但仍 attach 输出流读取 READY |
| CPU / 内存绑定 | `cpusetCpus`、`cpusetMems` 均为空；没有建立受控的 Local-NUMA 实验布局 |

当前仓库后续的 README / main 合并提交不是这次测量版本。检查显示，当前版本与测量版本之间的 `cmd`、`internal`、`probe` 内容没有差异。

### 实际部署中的特殊处理

根据当时的执行记录，安装的是 Ubuntu 发行版的 `docker.io` 软件包，不是 Docker 官方 `docker-ce` 软件包。安装记录还出现 containerd 2.2.1、runc 1.3.4；这些版本没有被结果包独立采集，应与结果包内可直接核查的 Docker 版本区分。

当时 Docker Hub 拉取 Alpine 超时，因此没有走默认 Dockerfile 的 Alpine/musl 构建路径，而是在 liuhui 用 GCC 12.3.0 对同一份 C probe 做静态编译，再将只含 probe 的 rootfs 导入 Docker。这是 **glibc 静态构建的 smoke 替代路径**，不是默认镜像构建方式。镜像构建和软件安装都发生在测量之外。

当时执行记录中的镜像 ID 为：

```text
sha256:36810f1cb22b0d86c5200c1a871551c7e8aedf054d50db1cf7a4f5ed07c66c4f
```

该 ID 未写入结果包的 manifest；结果包仅保存了镜像 tag。因此，结果包本身还不能完整证明镜像内容。后续若要比较 Hygon 与 AMD，应使用相同且锁定的 probe 镜像，不能将这次 glibc 版本与另一台机器的 musl 版本直接混为一组。

## 3. Smoke 实际做了什么

### 3.1 执行顺序与样本量

先做小规模 canary，确认基础链路正常，再执行正式 smoke。两次运行分别保存结果，不合并计算百分位。

| 运行 | 阶段 | 配置 / 实际行为 | 成功样本数 | 记录覆盖时长 |
|---|---|---|---:|---:|
| Canary | Warm-up | 至少 2 次且至少 1 秒，实际完成 5 次 | 5 | 1.020 s |
| Canary | Serial latency | 串行执行 10 次 | 10 | 1.901 s |
| Canary | Capacity，1 worker | 发起新循环的窗口为 2 秒 | 11 | 2.081 s |
| Canary | Capacity，2 workers | 发起新循环的窗口为 2 秒 | 20 | 2.046 s |
| 正式 smoke | Warm-up | 至少 100 次且至少 30 秒，实际完成 160 次 | 160 | 30.059 s |
| 正式 smoke | Serial latency | 串行执行 1,000 次 | 1,000 | 201.002 s |
| 正式 smoke | Capacity，1 worker | 发起新循环的窗口为 30 秒 | 159 | 30.088 s |
| 正式 smoke | Capacity，2 workers | 同上 | 288 | 30.009 s |
| 正式 smoke | Capacity，4 workers | 同上 | 510 | 30.164 s |
| 正式 smoke | Capacity，8 workers | 同上 | 792 | 30.206 s |

表中所有阶段的失败数都是 0。正式 smoke 的 2,909 条记录由 160 条预热、1,000 条串行测量和 1,749 条 capacity 测量组成；剔除预热后是 2,749 条测量记录。

Canary 的样本时间约为 12:54:38–12:54:45。正式 smoke 的样本时间约为 **12:54:53–13:00:45**，首条开始至末条结束约 **351.53 秒（5 分 52 秒）**，不包含安装及构建时间。

Capacity 是 closed-loop：每个 Go worker 完成上一轮后再开始下一轮；到达截止时间后不再发起新轮次，但等待已经在途的轮次结束。所以实际覆盖时长可能超过 30 秒。这里没有独立设定“每秒到达多少请求”，没有执行 offered-load 测试。

预热停止条件是“次数与时长都达标”，不是自动检测性能收敛。因此本次不是“固定只预热了 100 次”，也不能由预热完成推导出系统已稳定。

### 3.2 每一次 lifecycle 的具体过程

```text
宿主机 Go dlbench
  └─ 通过 Docker API 操作容器
       create → attach stdout/stderr → start → 读取 READY
                                               ↓
       remove ← wait 确认不再运行 ← SIGKILL
```

1. 创建一个使用 probe 镜像的新容器，并添加本次运行和样本的归属标签。
2. 在启动前接通输出流，避免漏掉 probe 的一次性 READY 通知。
3. 启动容器。静态 C probe 作为容器 PID 1，在 `main` 早期读取 `CLOCK_MONOTONIC_RAW`，向 stdout 输出一个 24-byte READY frame，随后等待被终止。
4. Go harness 读取 frame，并检查 probe 时间戳是否位于宿主机本次启动至收到 READY 的时间区间内。
5. 注册退出等待，发出 `SIGKILL`，等待 Docker 报告容器已不再运行。
6. 调用 remove，等 API 确认成功后，记录这一轮的时间和状态。

READY 在这里表示“这个极简 probe 已进入指定代码位置”，不是 Web 服务能处理请求，更不是模型或 agent 已加载完成。SIGKILL 测的是强制终止，不是 graceful shutdown。

本次计时、并发控制、Docker API 调用和记录都由 Go 完成，没有把 Python 纳入计时路径，也没有逐轮启动 `docker` CLI。Python wrapper 并非此次 smoke 的必要运行组件；本报告附带的 Python 脚本只用于事后复核数据。

每轮配置超时为 30 秒。按当时执行后的标签检查，benchmark 管理的残留容器数量为 0；本报告没有再次登录服务器核查当前状态。

## 4. 拿到了哪些性能数据

### 4.1 串行延迟：1,000 个正式样本

以下单位均为 **毫秒（ms）**。p50 是中位数，p95 / p99 是较慢尾部对应的百分位，按 nearest-rank 方法计算；不是均值或置信区间。

| 指标 | 计时边界与含义 | p50 | p95 | p99 |
|---|---|---:|---:|---:|
| `createNs` | 发起 create API → API 返回 | 27.71 | 44.92 | 58.61 |
| `startApiNs` | 发起 start API → API 返回 | 108.95 | 130.46 | 137.46 |
| `startToReadyNs` | 发起 start API → probe 内部 READY 时间戳 | 107.54 | 129.23 | 134.97 |
| `readyObservedNs` | 发起 start API → harness 完成读取 READY | 108.97 | 130.48 | 137.47 |
| `killApiNs` | 发起 kill API → API 返回 | 50.41 | 61.91 | 66.26 |
| `destroyAckNs` | 发起 kill API → wait 确认不再运行 | 50.42 | 61.91 | 66.27 |
| `removeAckNs` | 发起 remove API → API 返回 | 6.95 | 8.64 | 11.71 |
| `cycleAckNs` | 本轮开始、create 前 → remove API 返回 | 196.53 | 233.84 | 247.47 |

`cycleAckNs` 和 `startToReadyNs` 的百分位直接见原始 summary；其余六项由原始 operations 复算。预热和 capacity 样本没有混入这个表。

解读时需要注意：

- **107.54 ms 不含 create**，只是已创建容器的 start-to-probe-ready；不能把它叫作“从零创建并启动的总耗时”。
- 几项指标存在重叠：`destroyAckNs` 包含 kill API 时间；三个 start/READY 指标也共享起点。不能把所有行加起来，更不能将各阶段 p50 相加当作总 p50。
- harness 在 start API 返回后读取 READY；因此 `readyObservedNs - startToReadyNs` 不只是管道传输开销，也可能包含等待 API 返回和调度的时间。
- 完整生命周期还包含 attach、等待注册及其他 harness 工作；attach 没有独立计时字段。raw 计时值使用 `CLOCK_MONOTONIC_RAW`，字段单位是 ns，但纳秒存储不代表纳秒级准确度。
- remove 返回表示 Docker API 确认删除，不证明内核、存储设备和所有后台回收工作都在这一时刻物理完成。

### 4.2 Capacity：并发增加时的表现

下表均为正式 smoke 的独立 capacity 阶段，不是上一节串行样本的重用。

| Workers | 完成数 | 完整生命周期/秒 | Cycle p50 ms | Cycle p95 ms | Cycle p99 ms | Start-to-ready p50 ms |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 159 | 5.28 | 187.63 | 202.27 | 224.27 | 105.35 |
| 2 | 288 | 9.60 | 205.61 | 233.18 | 274.66 | 112.36 |
| 4 | 510 | 16.91 | 233.15 | 265.75 | 276.78 | 124.90 |
| 8 | 792 | 26.22 | 300.50 | 351.79 | 384.04 | 156.58 |

吞吐量公式为：成功轮数 ÷（该组最后一条 `finishedAt` − 第一条 `startedAt`）。分母使用记录中的实际覆盖时间，而不是一律使用配置的 30 秒。串行 latency 阶段也有吞吐量统计，为 4.98 次/秒；它与 capacity/1 的 5.28 次/秒来自不同时间段，不要求相同。

从 1 到 8 个 worker，吞吐量约增长 4.96 倍，同时单轮延迟上升。这说明在这几个测试点上，并发带来了更多完成量及更长单轮耗时；**不说明 26.22 次/秒就是机器最大容量，也不能据此断言 CPU、NUMA、存储或 dockerd 中哪一项是瓶颈**。

worker 数表示同时推进生命周期的逻辑工作流数，不表示固定运行容器数，更不是 CPU 核数。本次未绑核，Docker daemon、containerd、shim 及 harness 都可能由内核调度到多个核上。因此，1-worker 测试也不是“单核性能测试”。

此外，本次 capacity 每轮都包含 attach 与 READY 探针路径，是 instrumented diagnostic throughput，不是设计文档中尚待单独实现/验证的 uninstrumented headline throughput。

## 5. 保存的数据、校验结果与缺失项

### 5.1 每个结果包保存了什么

| 文件 | 内容 |
|---|---|
| `operations.ndjson` | 每次 lifecycle 一行：run/sample/worker/container ID、phase、并发度、开始/结束时间、八个延迟字段、probe 原始时间戳与 PID、状态及错误字段（错误时） |
| `summary.json` | 按阶段与并发度分组的尝试数、成功数、失败率、覆盖时长、吞吐量，以及 cycle / start-to-ready 的 p50、p95、p99 |
| `environment.json` | 裸机与 Docker 检查、OS/kernel/Docker/cgroup/storage/security 信息、CPU 与 NUMA 原始拓扑输出 |
| `campaign.normalized.json` | 实际执行的规范化配置，包括样本数、预热、并发、超时及 cpuset |
| `manifest.json` | 工具版本、源码 commit、Go 版本、OS/架构 |
| `status.json` | 整次运行最终状态及结束时间；两次均为 `SUCCESS` |
| `checksums.sha256` | 以上六个文件的 SHA-256 校验值 |

本次报告重新检查了两套包的全部 **12 个文件校验值**，均匹配；从 operations 重算的分组样本数、时长、吞吐量和已保存百分位也全部匹配。所有记录的 sample ID 和 container ID 在各自运行内唯一，probe PID 均为 1，延迟字段为正，基础时间顺序检查通过。

这些校验支持“当前保存的文件与其校验清单一致、统计可复算”，不是外部数字签名或独立真实性证明。没有失败记录也只代表此次有限测试，没有证明任意压力或任意运行时长下都不会失败。

### 5.2 没有采集或尚不能回答的内容

- 没有 perf/PMU、IPC、cache miss、CPU 利用率、上下文切换、磁盘 IO、温度/功耗或连续频率监控；**不能分类具体性能瓶颈**。
- 没有 Local-NUMA / Remote-client 的受控对比；记录 NUMA 拓扑不等于做了 NUMA 实验。
- 没有 offered-load 的到达率、排队、拒绝或丢弃测量，也没有比 8 workers 更高的容量扫描。
- 没有 daemon 冷启动、镜像拉取/解压耗时、真实应用初始化、网络服务 ready 或 graceful-stop 数据。
- 没有重复 campaign、随机化并发点顺序、收敛判定或置信区间；不报告 p99.9。各并发点连续执行，没有各自独立预热。
- 没有逐轮退出码字段，也没有单独的 attach 延迟和后台回收完成时刻；“成功”按目前代码中的 API/READY/等待/删除路径判定。
- 吞吐量覆盖时间来自持久化的 UTC wall-clock 字段，当前没有 wall-clock 跳变检测；与 raw 单调时钟测得的阶段延迟应区分。

对于“先在 7490H 跑通一次”的当前目标，这些缺失不妨碍 smoke 成功；只有准备做正式可比性能结论时，才需要补充相应控制与证据。

## 6. 能否运行在 AMD 上？

### 6.1 代码与构建层面：可以，条件是原生 x86-64 Linux 环境

检查 [环境检查代码][doctor-code]、[Docker 执行代码][engine-code] 和交付源码包内 `prebuilt/dlbench` 的构建元数据后，可以确认：

- 环境检查约束的是 Linux、`GOARCH=amd64`、裸机、cgroup v2、可访问的 rootful Docker；**没有检查 CPU vendor 必须为 Hygon**。提示文字虽包含 Hygon，但判断逻辑没有厂商限制。
- 交付包中的 Go 程序记录 `GOOS=linux`、`GOARCH=amd64`、`GOAMD64=v1`、`CGO_ENABLED=0`，以及正确的测试 commit、`vcs.modified=false`。
- `amd64` 在这里是 x86-64 架构名称，不是 AMD 厂商专用开关。Go 的 `GOAMD64=v1` 是通用基础指令集目标；更高的 v2/v3/v4 才提高指令集要求。参见 [Go 官方架构最低要求](https://go.dev/wiki/MinimumRequirements)。
- C probe 没有 Hygon 专用代码，既有构建命令未使用 `-march=native`。程序和镜像必须保持原生 linux/amd64，不能通过 QEMU 模拟来冒充跨平台测量。
- Docker 官方 Ubuntu 安装说明支持 x86_64/amd64，并列出 Ubuntu 22.04 LTS；是否能达到与 liuhui 相同软件栈仍需检查具体主机。参见 [Docker 官方 Ubuntu 支持范围](https://docs.docker.com/engine/install/ubuntu/)。

所以，更换为 AMD EPYC 等 x86-64 处理器并不要求重写计时引擎。本次实现仍仅检查 linux/amd64，不能由此扩展宣称“当前已支持所有处理器架构”，例如 ARM64 尚未在这一 smoke 路径中放行。

### 6.2 AMD 主机的最低核对清单（本次未执行）

1. 原生 Linux x86-64 裸机，具备所需系统接口与 cgroup v2；不是 Docker Desktop VM 或云 VM。
2. 可使用兼容的 rootful Docker Engine，运行用户有权访问其 socket；rootful 不等于 benchmark 操作者必须直接使用 root 账号。
3. 准备匹配的 linux/amd64 `dlbench` 和预装 probe 镜像，确保镜像架构一致。
4. 具备基础环境检查工具，尤其 `systemd-detect-virt`；`lscpu` 用于记录 CPU，建议安装 `numactl` 记录 NUMA。
5. 有足够的可用存储与结果目录写权限，并确认测试不会干扰同机重要任务。

若使用预编译程序和预制镜像，运行时不需要 Go/GCC 编译器，Python wrapper 也不是必需品；只有选择源码构建时才需要相应构建工具。后续 AMD 的运行名称和结果目录应单独标记，不能继续冒用 Hygon 的平台标签。

“能运行”与“能公平比较”是两回事。若以后要比较 AMD 与 Hygon，应固定源码、probe 镜像及构建方式，匹配或明确分组软件栈、容器配置和安全配置，并记录/控制 NUMA、SMT、频率与后台负载。**这些是后续比较要求，本次没有开展 AMD 测试。**

## 7. 文件位置与复核方法

本地正式结果：[hygon-7490h-smoke-20261016T045453Z-2fd62db1][smoke]。

本地 canary 结果：[hygon-7490h-canary-20261016T045437Z-29d78233][canary]。

liuhui 上当时保存并迁移后的路径为：

```text
/home/higon/yifan/dlbench-source-25bd3fe9a7f9/results/hygon-7490h-smoke-20261016T045453Z-2fd62db1
/home/higon/yifan/dlbench-source-25bd3fe9a7f9/results-canary/hygon-7490h-canary-20261016T045437Z-29d78233
```

报告配套的 [只读复核脚本][verify] 使用 Python 标准库，校验结果文件、重算百分位和吞吐量、读取交付包的 Go 构建元数据。它不会调用 Docker、发起 SSH 或创建测试容器：

```sh
python3 /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/verify-smoke-report.py
```

交付源码归档的 SHA-256 已核对匹配：

```text
归档：dlbench-source-25bd3fe9a7f9.tar.gz
SHA-256：3e7d615ca573703803c9aa53eb1385500956dd8de72124007f5cf49345cd71c7

归档内 prebuilt/dlbench：
SHA-256：d2139e9de5eb2461945c62375eafd1b4b88ac4cf6ae264c5e4c46d229d690f4b
```

注意：本地另有 `dist/linux-amd64/dlbench` 旧构建，其内嵌 VCS 信息是更早的 commit 且 `vcs.modified=true`，并不等于上述交付包中的二进制。本报告的构建兼容性证据取自**交付源码归档内的 prebuilt 文件**。结果 manifest 没有记录运行二进制本身的 SHA-256，因此仍需区分交付包校验与运行时身份的完整证明。

方法依据：[Phase 0 规范][phase0]、[实际执行顺序][runner-code]、[计时边界][engine-code]、[统计实现][summary-code]、[probe 源码][probe-code]。本报告只新增分析材料，没有修改 benchmark 实现、既有设计文档或原始测量文件，也没有提交或推送到 GitHub。

[environment]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/hygon-7490h-smoke-20261016T045453Z-2fd62db1/environment.json
[manifest]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/hygon-7490h-smoke-20261016T045453Z-2fd62db1/manifest.json
[campaign]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/hygon-7490h-smoke-20261016T045453Z-2fd62db1/campaign.normalized.json
[smoke]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/hygon-7490h-smoke-20261016T045453Z-2fd62db1
[canary]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/hygon-7490h-canary-20261016T045437Z-29d78233
[verify]: /Users/yifan/Desktop/workspace/docker-lifecycle/dist/liuhui-results/verify-smoke-report.py
[doctor-code]: /Users/yifan/Desktop/workspace/docker-lifecycle/internal/doctor/doctor.go
[engine-code]: /Users/yifan/Desktop/workspace/docker-lifecycle/internal/dockerengine/engine.go
[runner-code]: /Users/yifan/Desktop/workspace/docker-lifecycle/internal/runner/runner.go
[summary-code]: /Users/yifan/Desktop/workspace/docker-lifecycle/internal/summary/summary.go
[probe-code]: /Users/yifan/Desktop/workspace/docker-lifecycle/probe/probe.c
[phase0]: /Users/yifan/Desktop/workspace/docker-lifecycle/docs/phase0-hygon-smoke.md
