# CPU 容器负载性能测试

## 1. 目标与结论先行

目标是在三种 CPU/平台上，用同一套软件栈和工作负载回答两个问题：

1. **单容器延迟**：一个容器从创建到用户态进程可运行、再到进程退出和对象彻底删除，各阶段分别花多久？
2. **持续吞吐量**：多个 worker 不断执行完整生命周期时，每秒能完成多少次，延迟在并发下如何恶化？

两类测试都要做。单线程不停执行 `docker run --rm` 只能得到一个混合后的串行周期时间，不能解释慢在何处，也不能压满大核数服务器。最终至少报告：

- 串行阶段 latency：p50、p95、p99、p99.9、均值和跨 trial 的 95% CI；
- 并发 full-cycle：cycles/s、p50/p99 cycle latency、失败率；
- 固定资源与整机两种横向对比，不能只比较三台插槽数、核数不同的整机吞吐量。

## 2. “启动”和“销毁”到底包含哪些阶段

Docker 官方把 `create` 和 `start` 明确定义为两个操作：`create` 会创建容器对象、可写层并准备配置，但不启动它；`docker run` 本质上至少组合了 create 和 start。Docker Engine 再通过 containerd 管理 create/start/stop 生命周期，默认使用 runc。[Docker create 文档](https://docs.docker.com/reference/cli/docker/container/create/)，[Docker alternative runtimes 文档](https://docs.docker.com/engine/daemon/alternative-runtimes/)

### 2.1 可精确观测的 API 边界

主测试不用反复启动 `docker` CLI 子进程，而是由一个固定版本的测试驱动程序通过本机 Unix socket 直接调用 Docker Engine API。这样不会把 CLI 进程创建、参数解析和输出格式化算进每次容器延迟。驱动程序和容器 probe 都使用同一台 host 的 `CLOCK_MONOTONIC_RAW`；pilot 阶段必须校验二者时基一致，且未启用会改变时基的 time namespace 配置。

```text
t0  发送 POST /containers/create
t1  create 返回容器 ID
t2  发送 POST /containers/{id}/start
t3  start 返回 204
t4  容器内静态 probe 写入“首个用户态 ready”时间戳
t5  发送 POST /containers/{id}/kill?signal=SIGKILL
t6  kill 返回 204
t7  预先挂起的 /wait 请求返回，确认进程已经退出
t8  发送 DELETE /containers/{id}，force=false
t9  remove 返回 204，容器对象及可写层已经删除
```

必须记录的指标：

| 指标 | 定义 | 它回答的问题 |
|---|---|---|
| `api_ping` | Docker `/_ping` 请求往返 | API/Unix socket/测试驱动本身的基线 |
| `create_latency` | `t1 - t0` | 对象、配置、可写层/快照等准备有多快 |
| `start_ack_latency` | `t3 - t2` | Docker 接受 start 到 Engine 返回的时间 |
| `start_to_ready` | `t4 - t2` | 从请求启动到容器首个用户态 probe 真正运行 |
| `run_to_ready` | `t4 - t0` | 用户常说的“Docker 冷启动”，但不含拉取镜像 |
| `kill_ack_latency` | `t6 - t5` | 发送强制终止到 API 返回 |
| `exit_latency` | `t7 - t5` | 发送 SIGKILL 到 Docker 确认进程退出 |
| `remove_latency` | `t9 - t8` | 删除已停止容器对象和可写层/快照 |
| `destroy_latency` | `t9 - t5` | 从请求终止到完全删除；这是本计划对“销毁”的定义 |
| `cycle_latency` | `t9 - t0` | 一次完整 disposable-container 生命周期 |

`start_ack` 和 `ready` 不保证先后间隔恒定，必须分别记录；API 返回只能代表 Docker 操作完成到相应边界，不能自动等同于应用 ready。

Docker 的实时事件流会报告 `create`、`start`、`kill`、`die`、`stop`、`destroy`，bridge 网络还会报告 `connect`/`disconnect`。测试驱动应在 trial 开始前持续订阅事件，并使用纳秒时间字段做顺序校验和辅助归因，但 headline latency 仍使用驱动端的单调时钟。[Docker events 文档](https://docs.docker.com/reference/cli/docker/system/events/)

### 2.2 内部大致经过的组件

```text
测试驱动 → Docker Engine API → dockerd → containerd → shim/runc
                                              ↓
                         cgroup、namespace、rootfs mount、security、process
                                              ↓
                                     容器用户态 probe
```

可概念性地分为：

1. **Create 内部**：API 解码/校验、镜像本地查找、容器元数据、可写层或 snapshot、网络/挂载配置准备。
2. **Start 内部**：rootfs 挂载、网络激活、containerd task/shim、OCI runtime 创建 cgroup 和 namespaces、应用安全配置、创建并启动容器首进程。
3. **Exit 内部**：发送信号、进程退出、runtime/task 与 namespace/cgroup 清理、网络断开。
4. **Remove 内部**：删除 Docker 容器对象、元数据和可写层/snapshot。

这些内部子步骤不是 Docker 公共 API 提供的独立计时点。基线测试只对上述 API 边界做可靠计时；若某一阶段在某个平台显著落后，再用 tracing/perf 做第二轮归因，不能先把 API 时间武断地分摊给某个内部组件。

### 2.3 为什么主测试不用 `docker stop`、`rm -f` 或 `run --rm`

- `stop` 先发送可配置的停止信号并等待应用退出，超时后才强杀。因此它混有应用信号处理和 timeout；只适合作为单独的“graceful stop”场景。[Docker stop/run 文档](https://docs.docker.com/reference/cli/docker/container/run/#stop-container-with-signal---stop-signal)
- `rm -f` 把终止和删除合并，无法拆出 exit 与 remove。
- `run --rm` 把 create、start、应用运行/退出和 remove 合并，适合最终 E2E 数字，不适合阶段诊断。
- 主测试使用 `kill(SIGKILL) → wait → remove(force=false)`，把应用行为从销毁时间中排除；另设可选 graceful-stop 试验。

## 3. 测试工作负载

### 3.1 固定的最小镜像

制作并冻结一个 `scratch` OCI/Docker 镜像，内含一个保守 `x86-64` ISA 编译的静态 C probe；不能用 `-march=native`。三台机器加载同一个 image tar，并校验完全相同的 digest。

probe 做两种模式：

- `block`：启动后立刻读取 `CLOCK_MONOTONIC_RAW`，把 ready 时间戳写入 host tmpfs 上预分配的共享记录，然后等待信号；用于分阶段 latency。
- `exit0`：只做最小初始化并立即返回 0；用于 `run-to-exit-and-remove` 的 E2E 校验。

ready 通道会增加一个 bind mount 和一次 tmpfs 写入，因此：

- headline full-cycle 同时跑一组不带 ready instrumentation 的最小场景；
- `start_to_ready` 只在带 probe 的分阶段场景报告；
- 单独校准 tmpfs 写入开销，并在报告中说明它是测量开销上界的一部分。

镜像在所有 trial 前预加载，API 固定为 pull-never；镜像 pull、解压和 registry 网络不属于本测试。

### 3.2 三组核心测试

#### L1：Create/Remove only

```text
create → remove（从未 start）
```

目的：隔离对象、元数据、可写层/snapshot 分配和释放。

#### L2：复用对象的 Start/Exit

```text
预先 create 一次
重复：start → ready → kill(SIGKILL) → wait
最后 remove 一次
```

目的：尽量隔离 runtime task、namespace/cgroup、容器进程启动与退出，避免每轮 create/remove。

#### L3：完整 disposable lifecycle

```text
重复：create → start → ready → kill(SIGKILL) → wait → remove
```

目的：得到分阶段 latency 和完整 `cycle_latency`，也是吞吐测试的基本事务。

### 3.3 两个网络 profile

| Profile | 配置 | 用途 |
|---|---|---|
| CPU-focused | `network=none`、`log-driver=none`、无 healthcheck、无 volume（ready tmpfs 除外） | 最少干扰地比较控制路径和 CPU/内核开销 |
| Docker-default-like | 默认 bridge、相同 nftables/iptables backend、相同安全配置；应用不输出日志 | 衡量 Docker 常见网络路径带来的真实生命周期成本 |

如需要贴近 AI agent sandbox，再增加第三个 profile：固定的 Python/Node agent 基础镜像、workspace bind mount、默认 bridge。它应作为独立结果，不能替代最小基线。

## 4. 延迟与吞吐实验设计

### 4.1 串行 latency

- 并发度固定为 1。
- 每个测试/profile 先 warm-up 至少 60 秒或 1,000 cycles，取更晚者。
- 正式记录 10,000 cycles；做 5 个独立 trial。
- trial 顺序随机化；trial 之间等待温度、CPU 频率和 I/O queue 回到稳定范围。
- 主要报告 p50/p95/p99/p99.9、均值、标准差、跨 trial 变异系数和 bootstrap 95% CI。
- 保留每次 operation 的原始 NDJSON/Parquet 记录，不只保存聚合值。

### 4.2 稳态吞吐量

采用 closed-loop worker：每个 worker 完成一次 L3 full-cycle 后才开始下一次，确保没有无界请求队列。

- 并发度建议：`1, 2, 4, 8, 16, 32, 64`，再按共同物理核数和整机规模扩展。
- 每点 warm-up 30 秒，正式测量 120 秒，cool-down 至少 30 秒；5 个 trial。
- 吞吐量定义为正式窗口内 `remove` 成功返回的 full cycles 数 / 实际窗口秒数。
- 同时报告 cycle p50/p99/p99.9、阶段 p99、错误/超时率和残留容器数。
- 找到吞吐拐点：增加并发后吞吐增益低于约 5%，或 p99/错误率明显跃升的位置。

不要把 concurrency=1 的无限循环称为“整机最大吞吐”；它只是串行生命周期 rate，约等于 `1 / 平均 cycle latency`。

### 4.3 Warm、daemon-cold 与真正 cold

主结果是 **warm daemon + local warm image**，对应 agent/sandbox 高频反复创建容器的场景。

可选补充：

- daemon-cold：重启 Docker 后只测最初若干次请求，单独报告；
- page-cache cold：只作为存储敏感性实验，不在每次主循环前 drop cache；
- image cold/pull：完全独立测试，因为它主要测 registry、网络、解压与存储，不是容器生命周期 CPU 对比。

## 5. 三个平台必须配置的环境

### 5.1 硬件与拓扑

三台都用 bare metal，禁止 VM/云主机。实验前保存：

- CPU 完整型号、stepping、microcode、BIOS/BMC 版本；
- socket 数、每 socket 物理核数、SMT、NUMA node 数和每 node CPU 列表；
- 内存通道填充方式、容量、频率；
- `/var/lib/docker` 所在 SSD/NVMe 型号、固件、文件系统和 mount options；
- 机器散热、功率上限和实验期间是否发生 thermal/power throttling。

CPU 名字相同不代表平台相同。若 9754、9755 和 7490H 机器的 socket 数、内存通道或 SSD 不同，必须把结论写成“平台对比”；只有固定核数/NUMA/软件/存储后，才能更接近“CPU 对比”。

### 5.2 推荐软件栈

| 项目 | 要求 |
|---|---|
| OS | 三台使用同一发行版、同一安装镜像；推荐选择三款平台都正式支持的同一 Ubuntu 24.04 LTS 或同一企业发行版 |
| Kernel | 完全相同的 kernel package/build 与 boot parameters |
| Docker | 同一 Docker Engine 精确版本和 build，不用模糊的 `latest` |
| containerd/runc | 精确版本一致，并记录 `docker version` 输出 |
| 运行模式 | rootful Docker；三台都使用 cgroup v2 |
| 存储栈 | 明确固定为同一种：Docker Engine 29+ 的 containerd image store + overlayfs snapshotter，或统一 classic overlay2；不能让新装/升级历史自动决定 |
| backing FS | 同为 ext4 或同为 XFS，mount options 相同，最好使用相同型号本地 NVMe |
| 安全 | seccomp、AppArmor/SELinux、capabilities、内核漏洞 mitigations 状态一致 |
| 网络 | bridge 测试的 nftables/iptables backend 与规则集一致 |
| 工具 | 同一个预编译测试驱动、同一个 probe 镜像 digest；另装 `perf`、`numactl`、`sysstat` 用于观测 |

Docker Engine 29.0 起，全新安装默认使用 containerd image store，而旧版本升级可能继续使用 classic storage driver；因此只看 Docker 版本仍可能混用不同存储路径，必须从 `docker info` 验证。[Docker storage drivers 文档](https://docs.docker.com/engine/storage/drivers/)

### 5.3 BIOS、调频与系统噪声

主 profile 建议：

- BIOS 选择 performance-oriented profile；三台都保持 boost/turbo 开启，保留产品真实动态加速能力；
- 使用相同 CPU frequency governor；禁用或统一 deep C-state 策略；
- SMT 主结果使用每物理核一个 hardware thread，另跑一组 SMT-on 敏感性测试；
- NUMA/NPS 设置记录清楚。不要假设三款平台能提供完全相同的 NPS 选项；比较时以 Linux 实际拓扑为准；
- 关闭无关定时任务、自动更新、扫描和其他用户负载；测试机保持专用；
- IRQ、kworker 和 benchmark cores 的位置固定；在主测试前验证没有明显 CPU migration；
- 保持安全 mitigations 开启且一致。若做 `mitigations=off`，只能作为单独敏感性实验。

### 5.4 两种 CPU/NUMA 配置都要测

#### A. Fixed-resource：回答“同样资源下谁更快”

- 选择三台共同可用的同一物理核数，例如 8 或 16 个物理核；最终数值由三台 `lscpu -e` 和 NUMA 拓扑决定。
- 核全部来自一个 NUMA node；内存也绑定到该 node。
- 预留 1 个物理核给测试驱动；给 dockerd/containerd/shim 固定相同数量的控制核；容器 worker 使用相同数量的独立物理核。
- 避开 SMT sibling 重叠，避免测试驱动和 Docker 控制面争用同一物理核。

#### B. Whole-system：回答“整台服务器能承载多少”

- 使用所有允许的 CPU/NUMA 资源，保持厂商推荐的生产设置。
- 分别报告 whole-system cycles/s、cycles/s/socket 和 cycles/s/physical-core。
- 原始整机吞吐只能用于平台容量比较，不能直接宣称某一 CPU 核心更快。

## 6. 观测与归因数据

headline 测试期间低频采集以下数据：

- dockerd、containerd、shim/runc 与测试驱动的 CPU time、context switches、CPU migrations、page faults；
- 每 CPU 利用率、run queue、内存和 NUMA 分配；
- `/var/lib/docker` 设备的 IOPS、吞吐、平均/p99 latency、queue depth；
- CPU 实际频率、温度、功率和 throttle 状态；
- Docker events 序列与事件数量；
- trial 前后的容器、snapshot/可写层数量，确保无资源泄漏。

PMU/perf 放在单独的 diagnostic rerun，避免给 headline 数字增加扰动。跨 AMD/Hygon 首先只用可移植事件和软件计数；厂商专用 L3/DF/分支事件只用于平台内归因，必须先通过 `perf list` 核对，不能假设事件编码通用。

当出现显著差异时按以下顺序定位：

1. 查看差异落在 create、start、exit 还是 remove；
2. 检查 NVMe/filesystem latency、CPU frequency、调度迁移和 NUMA；
3. 对落后阶段单独运行 L1 或 L2；
4. 再对 dockerd/containerd/runc 做 on-CPU/off-CPU profiling 和必要的 eBPF tracing；
5. 最后才用厂商 PMU 解释微架构原因。

## 7. 结果有效性门槛

满足以下条件才接受一个 trial：

- create/start/kill/die-or-wait/destroy 数量匹配，无残留 benchmark 容器；
- operation 错误率为 0；吞吐极限点若出现错误，则保留并明确报告饱和/失败边界；
- 未发生 image pull、daemon restart、OOM、thermal/power throttle；
- background load、CPU migration 和存储 queue 未出现异常尖峰；
- 软件版本、镜像 digest、Docker storage backend、网络和安全配置与冻结清单一致；
- 同配置的 5 个 trial 之间无无法解释的系统性漂移。

## 8. 实施顺序与交付物

1. **环境冻结**：三台采集统一 manifest，确认 OS/kernel/Docker/containerd/runc、cgroup、storage、网络、BIOS 和 NUMA。
2. **构建测试资产**：一个静态 probe 镜像、一个直接调用 Engine API 的测试驱动、固定配置文件。
3. **功能校验**：每阶段事件顺序正确，ready 时钟可比较，异常中断能按本次 run label 精确清理。
4. **pilot**：每台先跑 L1/L2/L3 各 1,000 次，确定超时、采样开销和并发上限。
5. **正式 latency**：CPU-focused 与 Docker-default-like，fixed-resource，5 × 10,000 cycles。
6. **正式 throughput**：fixed-resource 和 whole-system，并发扫描，每点 5 × 120 秒。
7. **针对性诊断**：只对出现显著差异的阶段追加 perf/eBPF；不改变 headline 运行配置。
8. **报告**：原始记录、环境 manifest、聚合 CSV/图表、阶段 waterfall、吞吐-并发曲线、p99-并发曲线和结论限制。

## 9. 建议的首版最小范围

如果要先在三台机器快速得到可信结果，首版只做：

- bare metal、同 OS/kernel/Docker/containerd/runc；
- rootful + cgroup v2；
- 同一种 Docker image store/snapshotter 和同等级 NVMe；
- 同一个 `scratch` 静态 probe 镜像；
- `network=none`、`log-driver=none`；
- fixed-resource 单 NUMA node、SMT 每核只用一个 thread；
- L1、L2、L3 串行 latency；
- L3 并发 `1/2/4/8/16/32/64` 的吞吐扫描；
- 主指标为 `create/start_to_ready/exit/remove/cycle` latency 和 cycles/s。

完成这组以后，再加默认 bridge、whole-system、SMT-on 和 agent 基础镜像，避免第一轮变量过多。
