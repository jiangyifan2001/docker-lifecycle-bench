# Hygon 7490H 单逻辑 CPU smoke 结果与完备性检查

报告日期：2026-10-22。主机：liuhui。下文时间均为北京时间（UTC+8）。

## 1. 结论

新版已覆盖部署到原仓库目录，完成了一次 **CPU 16 单逻辑 CPU smoke**，最终状态为 `SUCCESS`。共记录 **1,753 个完整生命周期，零失败**：143 次 warm-up、1,000 次 serial、610 次 concurrent。另有 1 个不计入统计的绑核 canary。

Serial E2E mean 为 **209.93 ms**，p99 为 **271.50 ms**。Concurrent 从 1 增至 8 workers，吞吐量仅从 **4.77 增至 5.17 cycles/s**，E2E mean 则从 **209.75 增至 1,535.21 ms**。这说明在本次固定单 CPU 预算下，增加在途生命周期主要增加等待时间，没有获得接近线性的吞吐收益。

**当前单核 smoke 路径已跑通并通过数据核验；完整 benchmark 设计尚未全部实现。** 本次不证明 CPU 独占、NUMA-local 内存、统计收敛、最大吞吐量或跨平台可比性，也没有运行 AMD 机器。

本地改动没有 commit，没有 push。

## 2. 部署、版本与备份

| 项目 | 实际值 |
|---|---|
| 本地仓库 | `/Users/yifan/Desktop/workspace/docker-lifecycle` |
| 远端仓库，原路径覆盖更新 | `/home/higon/yifan/dlbench-source-25bd3fe9a7f9` |
| 实际运行二进制 | 上述目录内 `prebuilt/dlbench` |
| 版本标识 | `0.1.0-single-cpu-dev`，`8ad8470-working-tree`，未提交工作区标识而非新 commit |
| 编译与执行 | Go 1.27.2；macOS 交叉编译 `linux/amd64`、`CGO_ENABLED=0`，liuhui 原生执行，无 QEMU |
| 本次运行及恢复材料 | `/home/higon/yifan/dlbench-single-cpu-20261022/` |
| 远端旧版完整备份 | 上述目录内 `previous-version.tar.gz`，包括旧代码、旧二进制与旧结果 |
| 本地改动前备份 | `/Users/yifan/Desktop/workspace/docker-lifecycle-before-single-cpu.8j2cKp/` 内 `source.tar.gz` 和 `history.bundle` |

原目录名保留了历史提交的短哈希，但里面现在是新版；实际版本应以二进制、`SOURCE_COMMIT` 和文件校验和为准。原有 smoke 结果未覆盖。

测量使用的二进制 SHA-256：

```text
1a633124815a4272ee8d4f5b456cd813551b629cccd938ac62cba4a5476699bd
```

部署时源码归档 `source.tar.gz` SHA-256：

```text
43c3194758a42171f86146ac30923c7b6149f38fb1b34925672ac625c26a31bf
```

旧版备份 SHA-256：

```text
a78b6ee06b92e6d6e7fc248f38cfea21083ac4340cd443b6aed08f2f2a73144d
```

恢复工具的兼容性修复和独立分析脚本在测量后补充同步；没有更换测量二进制，也没有改写原始结果包。

## 3. 环境与“全绑单核”的实际范围

系统为 Ubuntu 22.04.5 LTS、Linux 5.15.0-119-generic、systemd 249、Docker 29.1.3、rootful Docker、cgroup v2 / systemd driver、runc。用户确认 liuhui 为 Hygon 7490H；系统 CPU 字符串保留为 `Hygon C86-4G/H (OPN:7446)`，不擅自改写型号。

主机有 192 个在线逻辑 CPU（0–191）、2 个 socket、12 个 NUMA 节点。本次选择在线 CPU 16：socket 0、core 16、NUMA node 1；其 `thread_siblings_list` 实际为 `16`。开始前 Docker 无容器，containerd 的已检查 namespace 无任务，主机 load average 为 0.00。

绑定证据来自 [placement.before.json][before] 与 [placement.after.json][after]：

| 对象 | 预检线程数 | 实际限制 | 验证方法 |
|---|---:|---|---|
| Go harness | 6 | 每个线程的 `Cpus_allowed_list=16`；`GOMAXPROCS=1` | 从绑核 OS 线程重新 exec；逐线程检查；测试前后核验 |
| dockerd，PID 308975 | 97 | cgroup effective CPU 16，所有所查线程允许 CPU 16 | 服务 cgroup 和线程状态；前后 PID 不变 |
| containerd，PID 308535 | 117 | cgroup effective CPU 16，所有所查线程允许 CPU 16 | 服务 cgroup 和线程状态；前后 PID 不变 |
| canary shim | 10 | cgroup 和所有线程允许 CPU 16 | 检查实际 probe 的父进程为 `containerd-shim-runc-v2` |
| canary probe | 1 | cgroup 和线程允许 CPU 16 | Docker inspect 的真实 host PID，且 scope 与容器 ID 匹配 |
| 正式测量中的容器 | 每轮创建 | `CpusetCpus=16`、`CgroupParent=dlbench.slice`、runtime `runc` | 与 canary 共用配置生成逻辑；父 slice effective CPU 16 |

Harness 的父 cgroup 本身仍允许 0–191，**不等于 harness 可以跨核**：harness 使用的是线程 affinity，每个被检查线程仅允许 CPU 16。Docker/runtime 则由 cgroup 限制未来任务可用的 CPU 范围。

短命 runc 进程依靠受限服务 cgroup 与容器父 cgroup 的继承规则约束，没有对每一个 runc exec 做调度追踪。测试前后验证也不能排除管理员在中途改配置又恢复的情况；本次执行流程未进行这种操作。

NUMA 内存策略没有改变，`cpusetMems` 为空，所记录 effective memory nodes 为 `0-11`。因此这不是 Local-NUMA 内存实验。没有设置 CPU 独占、IRQ 隔离或系统后台任务隔离。

本次锁定并复用了 10 月 16 日 smoke 的同一 native probe 镜像，没有重新拉取或构建：

```text
sha256:36810f1cb22b0d86c5200c1a871551c7e8aedf054d50db1cf7a4f5ed07c66c4f
```

## 4. 执行顺序与结果

执行顺序为：环境检查 → 镜像检查 → 绑核预检 → 1 个独立 canary → warm-up → serial → concurrent 1/2/4/8 → 绑核后检 → 完成结果包。

Warm-up 同时满足至少 100 次和至少 30 秒，实际 143 次、30.077 秒。原始采样覆盖 **15:27:20.337–15:33:21.617**，约 361.28 秒。Concurrent 每点发起窗口为 30 秒，截止后等待在途 cycles 完成，因此实际统计窗口略长。

E2E 即 `cycleAckNs`：本轮 create 前开始，到 remove API 返回为止。不是 Python wrapper、整个 campaign 或物理存储回收的耗时。下表不包含 warm-up。

| Test phase | Workers | # of cycles | E2E mean (ms) | E2E p50 (ms) | E2E p95 (ms) | E2E p99 (ms) | Cycles/s |
|---|---:|---:|---:|---:|---:|---:|---:|
| Serial latency | 1 | 1,000 | 209.93 | 209.75 | 243.65 | 271.50 | 4.76 |
| Concurrent | 1 | 143 | 209.75 | 209.86 | 238.40 | 261.86 | 4.77 |
| Concurrent | 2 | 153 | 393.07 | 385.63 | 465.84 | 674.04 | 5.08 |
| Concurrent | 4 | 156 | 780.18 | 788.05 | 916.66 | 972.48 | 5.11 |
| Concurrent | 8 | 158 | 1,535.21 | 1,551.97 | 1,803.73 | 1,883.07 | 5.17 |

Concurrent 实际窗口分别为 30.004、30.109、30.507、30.587 秒。吞吐量使用成功次数除以实际观测窗口，不是直接除以配置中的 30 秒。Serial 与 Concurrent-1 是独立阶段，不复用样本。

### 串行样本的分阶段 latency

以下全部来自该次运行的 1,000 个 serial samples，单位 ms。

| Metric | Mean | p50 | p95 | p99 |
|---|---:|---:|---:|---:|
| createNs | 22.72 | 21.27 | 34.56 | 48.29 |
| startApiNs | 126.38 | 128.97 | 157.07 | 173.10 |
| startToReadyNs | 118.13 | 121.00 | 146.61 | 159.50 |
| readyObservedNs | 126.38 | 128.98 | 157.08 | 173.12 |
| killApiNs | 53.75 | 50.12 | 77.61 | 101.03 |
| destroyAckNs | 53.82 | 50.16 | 77.65 | 101.04 |
| removeAckNs | 5.66 | 5.09 | 6.03 | 22.39 |
| cycleAckNs / E2E | 209.93 | 209.75 | 243.65 | 271.50 |

这些指标存在嵌套或重叠，不能逐行相加。Attach 没有独立计时字段，但包含在完整 E2E 内。Mean 从原始样本算术平均获得，不是对分位数取平均。

### 与上次 smoke 的关系

上次未建立单核限制的 Concurrent-8 为 26.22 cycles/s、E2E mean 303.84 ms；本次为 5.17 cycles/s、1,535.21 ms。两者是不同 CPU 资源约束下的结果，不能混用为同一配置。

日期、harness 构建版本和运行时设置也有变化，没有做同日交替重复的严格 A/B。因此不能将全部差异精确归因于某一个硬件机制。本次没有 perf/PMU 或连续 CPU 利用率证据，不宣称“已定位为某种微架构瓶颈”。

## 5. 完备性检查

| 检查项 | 结果与证据 |
|---|---|
| 单核参数与默认兼容性 | Go `run/validate --single-cpu`、YAML、Python 透传已实现；不传参数保持原路径；CPU 0、负值和冲突配置有测试 |
| 环境未准备时拒绝运行 | 实机负向测试返回 `BLOCKED` / exit 3，原始 samples 为 0，没有偷偷修改 daemon 或退回多核 |
| Harness 新线程继承 | liuhui 原生运行 Linux 测试：子进程重新 exec 后，新建 8 个锁定 OS 线程，逐线程核验通过 |
| Docker/probe/shim 限制 | 实机 canary 及前后 host 检查通过；绑定失败、scope 不匹配、未绑定 shim 的测试覆盖 |
| 生命周期与调度 | 1,753 条成功记录；样本 ID 连续唯一，容器 ID 唯一，probe PID 均为 1；阶段顺序及同一 worker 内不重叠通过 |
| 指标完整性 | 每条成功样本的 8 项 latency 均为正；readiness、kill/wait 顺序与 E2E 覆盖关系通过 |
| 汇总可信度 | 独立 Python 实现重算所有组的计数、E2E/start-to-ready p50/p95/p99、吞吐量，与 Go summary 一致；另算 E2E mean |
| 原始结果完整性 | 8 个文件 SHA-256 校验通过；本地下载副本再次通过独立校验 |
| 清理与权限 | canary 取消/失败/清理失败/所有权不匹配有测试；实机结束后 Docker 无残留容器；普通 run 无自动 sudo 或服务重启 |
| 开发验证 | 本地 `go test -race ./...`、`go vet ./...`、Linux 交叉编译通过；liuhui 原生 placement/dockerengine 测试通过；6 项 Python 测试通过 |
| 环境恢复 | Docker/containerd 实际 CPU 范围恢复 0–191，主进程 PID 不变；专用 slice 停止；旧内核兼容细节见下一节 |

独立校验输出见 [verification.json][verification]，校验器为 [scripts/verify-result.py](../../scripts/verify-result.py)。这不是“所有代码路径均被证明正确”：CLI、doctor、真实 Docker 异常路径等仍没有完整自动化覆盖；本次实机只验证正常路径及未准备环境的拒绝路径。

### 距离完整 benchmark 还缺什么

1. **统计层面**：只有一次 smoke，Concurrent 每点仅 143–158 个 samples；按 nearest-rank 算的 p99 在这些点上基本是第二大样本。正式尾延迟结论需要更长测量、多次独立重复、稳定性判断和置信区间。本次不提供 p99.9。
2. **环境控制**：还没有独占核、频率/温度连续采集、CPU/IRQ 背景噪声控制及 Local/Remote NUMA 内存布局；不能称为“纯 CPU 单核性能”。
3. **功能范围**：当前实现 instrumented FullLifecycle + forced teardown + closed-loop concurrent；offered-load、其他 lifecycle scenarios、graceful teardown、uninstrumented headline 等完整设计项仍未实现。
4. **证据粒度**：绑核是约束及前后快照，不是全程 scheduler trace；短命 runc 的继承路径没有逐次追踪。阶段时间有重叠，Attach 无独立字段；Remove 是 API acknowledgement，不证明物理资源已经回收。
5. **跨平台复现**：没有在 AMD 或其他 ISA 上复验；比较前需冻结镜像、Docker/runtime、harness 构建及环境策略，再做一致的配置实验。

当前结论应限定为：**在 liuhui 上，单逻辑 CPU 参数、约束验证、完整生命周期运行、采样汇总和清理路径可用。**

## 6. 恢复过程中发现并处理的问题

这次实机检查发现，仅执行 `systemctl set-property --runtime ... AllowedCPUs=` 不足以证明恢复完成：配置属性已空，但两个服务的 kernel effective cpuset 仍为 `16`。

随后尝试清空对应 `cpuset.cpus`，主机内核返回 `ENOSPC`。磁盘仍有约 697 GiB 可用，这是 cpuset 对有任务组的限制，不是存储耗尽。[内核关于放开该限制的补丁讨论](https://lkml.indiana.edu/hypermail/linux/kernel/2104.1/06797.html)解释了该类行为；本报告以实机返回值为依据，不假定补丁已经包含在此主机内核中。

恢复工具已补充兼容处理和回归测试：在不能清空时，显式恢复原有效 CPU 列表并核验。最终实际状态为：

- dockerd / containerd：`EffectiveCPUs=0-191`，主线程 `Cpus_allowed_list=0-191`，原 MainPID 保持不变；没有重启服务。
- `AllowedCPUs` 配置属性为空；内核中是显式 `0-191` 列表，**没有残留单核限制**，但不等同于空列表继承模式。若将来热添加 CPU 或要求彻底恢复空列表，可在管理员批准的服务维护窗口处理。
- `dlbench.slice` 已停止；Docker 容器列表为空。

这项修复属于管理员准备/恢复工具，不改变本次计时数据。恢复状态记录保存在 `host-state.json`，实际恢复后的服务状态见 [host-after.txt][host-after]。

## 7. 结果位置与复跑

正式结果包远端路径：

```text
/home/higon/yifan/dlbench-single-cpu-20261022/results/
  hygon-7490h-single-cpu-smoke-20261022T072719Z-520a46b2/
```

同目录另一个 `...T072628Z-d2cc137a` 是预期的未准备环境负向测试，不是性能测量失败样本。

本地副本位于 `dist/liuhui-results/single-cpu-20261022/`。独立核验命令：

```sh
python3 scripts/verify-result.py \
  dist/liuhui-results/single-cpu-20261022/results/hygon-7490h-single-cpu-smoke-20261022T072719Z-520a46b2
```

复跑需要重新进行明确授权的管理员准备；当前服务已经恢复多 CPU 范围，直接传单核参数会被拒绝，而不会自动重新修改服务。完整步骤见 [single-cpu runbook](../single-cpu.md)。

[before]: ../../dist/liuhui-results/single-cpu-20261022/results/hygon-7490h-single-cpu-smoke-20261022T072719Z-520a46b2/placement.before.json
[after]: ../../dist/liuhui-results/single-cpu-20261022/results/hygon-7490h-single-cpu-smoke-20261022T072719Z-520a46b2/placement.after.json
[verification]: ../../dist/liuhui-results/single-cpu-20261022/verification.json
[host-after]: ../../dist/liuhui-results/single-cpu-20261022/host-after.txt
