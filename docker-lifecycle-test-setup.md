# 跨平台 CPU 容器负载测试：环境装配与验收手册

适用平台：

- AMD EPYC 9754
- AMD EPYC 9755
- Hygon 7490H

测试对象是 CPU/服务器平台；Docker create/start/exit/remove 是系统级 CPU 负载。本文只负责装配统一测试环境，不负责运行正式测试。

## 0. 给执行 Agent 的强制规则

1. **先完成只读审计，再做任何修改。**
2. **不得自动重装系统、修改 BIOS、刷新固件、修改 GRUB、分区、格式化或删除数据。**这些操作必须由机器所有者单独授权。
3. 如果发现已有容器、镜像、独立安装的 containerd/runc、Kubernetes、Podman，或者 `/var/lib/docker`、`/var/lib/containerd` 中已有数据，立即停止；不得卸载、迁移、覆盖或清理。
4. 不得为了“装成功”而让三台使用不同 OS、kernel、Docker storage backend 或安全配置。
5. 不得安装 `latest` 浮动版本。先在三台机器上发现可用版本，再由协调者冻结精确版本。
6. 所有失败都应保留原始输出并回报，不得通过关闭安全机制、扩大权限或使用非官方安装脚本绕过。
7. 每台机器最后只能返回 `READY` 或 `BLOCKED`。任何验收项失败都应返回 `BLOCKED`，不能标成“基本可用”。

## 1. 冻结的共同基线

### 1.1 推荐基线

| 项目 | 统一要求 |
|---|---|
| 机器 | 独占 bare metal x86_64；OS 直接运行在硬件上 |
| OS | Ubuntu Server 24.04 LTS amd64，三台使用同一个 ISO/镜像及 SHA-256 |
| Kernel | 三台 `uname -r` 完全相同；使用同一仓库的同一 kernel build |
| Docker | 官方 Docker CE apt 仓库；选择三台共同可用的同一 Engine 29.x 稳定精确版本 |
| containerd/runc | 使用 Docker 官方 `containerd.io` 包所带版本；三台精确一致 |
| Docker 模式 | rootful，Unix socket，不使用 Docker Desktop/Rootless/远程 daemon |
| cgroup | cgroup v2 + systemd cgroup driver |
| 镜像存储 | containerd image store + overlayfs snapshotter |
| Docker 数据盘 | 本地 NVMe；`/var/lib/docker` 和 `/var/lib/containerd` 位于经过确认的相同存储类型上 |
| 文件系统 | 三台一致，推荐 ext4；格式及 mount options 一致 |
| 安全 | 默认 seccomp；AppArmor、kernel mitigations 等状态一致且开启 |
| 时间 | 同一时区策略、NTP/chrony 同步正常；正式 latency 使用 monotonic clock |
| 测试账户 | 专用本地账户，例如 `dockerbench`；允许访问 `/var/run/docker.sock` |

Docker 官方当前支持 Ubuntu 24.04 LTS amd64；Ubuntu 衍生发行版不属于官方验证范围。[Docker Ubuntu 安装文档](https://docs.docker.com/engine/install/ubuntu/)

如果任意平台不能使用同一 Ubuntu 镜像和同一 kernel，不要给该平台单独安装其他发行版。先回报兼容性问题并重新选择三台共同支持的发行版。

### 1.2 安装前必须填写的版本锁

协调者必须先根据三台机器的审计结果填写下表，再允许进入安装阶段：

| 锁定项 | 冻结值 |
|---|---|
| Ubuntu ISO/镜像名称 | `TBD` |
| Ubuntu ISO SHA-256 | `TBD` |
| `uname -r` | `TBD` |
| `docker-ce` apt version | `TBD` |
| `docker-ce-cli` apt version | `TBD` |
| `containerd.io` apt version | `TBD` |
| Docker Engine API version | 安装后记录 |
| Docker storage backend | `containerd image store` |
| containerd snapshotter | `overlayfs` |
| Docker backing filesystem | `TBD`，三台相同 |
| Docker 数据盘型号/等级 | `TBD` |
| Benchmark 用户 | `dockerbench` 或明确的其他账户 |

任一 `TBD` 未填写时，不得安装 Docker。

## 2. 第一阶段：只读准入审计

在每台机器执行下列只读检查，保留完整输出。不要只复制摘要。

### 2.1 硬件、OS 与虚拟化

```bash
hostnamectl
cat /etc/os-release
uname -a
uname -r
dpkg --print-architecture
systemd-detect-virt
lscpu
lscpu -e=CPU,CORE,SOCKET,NODE,ONLINE,MAXMHZ,MINMHZ
grep -m1 -w hypervisor /proc/cpuinfo
sudo dmesg | grep -i -E 'hypervisor|virtualization|microcode|pmu'
```

准入条件：

- `dpkg --print-architecture` 为 `amd64`；
- `systemd-detect-virt` 返回 `none`；
- `lscpu` 中没有 Hypervisor vendor；
- `/proc/cpuinfo` 没有 `hypervisor` flag；
- 有机器来源或供应商证据确认它是独占物理机。

云厂商真正的 bare-metal instance 可以进入候选；Dedicated Host 上的 VM、普通云实例、KVM/VMware、Docker Desktop 和 WSL2 都不合格。

### 2.2 CPU、NUMA 与内存

```bash
numactl --hardware
lscpu --extended
free -h
sudo dmidecode --type 4
sudo dmidecode --type 17
cat /proc/cmdline
```

记录：

- socket、物理核、thread、SMT sibling；
- NUMA node 数和每 node CPU 列表；
- 内存通道填充、DIMM 容量和速率；
- BIOS、BMC、microcode 版本；
- NPS/NUMA、功率与 SMT 策略。

不同 CPU 平台的内存代际可能不同，这是无法消除的平台差异；仍需保证每台机器通道对称填充，并在结果中称为“系统级 CPU/平台性能”。

### 2.3 本地存储

```bash
lsblk -e7 -o NAME,MODEL,SERIAL,FSTYPE,FSVER,SIZE,ROTA,MOUNTPOINTS
findmnt -T /var/lib
df -hT
sudo nvme list
```

如果 `/var/lib/docker` 或 `/var/lib/containerd` 已存在，再执行：

```bash
sudo findmnt -T /var/lib/docker
sudo findmnt -T /var/lib/containerd
sudo du -sh /var/lib/docker
sudo du -sh /var/lib/containerd
```

准入条件：

- Docker 数据必须落在本地 SSD/NVMe，不能使用 NFS、Ceph、SAN 或其他网络/共享存储；
- 三台尽量使用同型号、同固件的 NVMe；最低要求是同等级设备和相同文件系统/mount options；
- 如果目录已有数据，停止，不得删除；
- 如果需要新分区或格式化，停止并请求机器所有者处理。

Docker Engine 29+ 使用 containerd image store 时，镜像和 snapshot 主要位于 `/var/lib/containerd`，并非只在 `/var/lib/docker`；两个位置都必须检查。[Docker daemon 数据目录文档](https://docs.docker.com/engine/daemon/)

### 2.4 已有容器软件与工作负载

```bash
dpkg -l docker-ce docker-ce-cli docker.io containerd containerd.io runc podman-docker
systemctl status docker --no-pager
systemctl status containerd --no-pager
systemctl status kubelet --no-pager
docker ps -a
docker images
podman ps -a
uptime
systemctl --failed
ps -eo pid,psr,comm,%cpu,%mem --sort=-%cpu
```

以下任一情况必须停止：

- 已有 Docker/Podman/Kubernetes 工作负载；
- 已安装来源不明或版本不一致的 containerd/runc；
- 机器被其他用户或服务持续占用；
- 无法确认卸载或重装不会影响现有数据。

### 2.5 cgroup、调频与 PMU

```bash
test -f /sys/fs/cgroup/cgroup.controllers
cat /sys/fs/cgroup/cgroup.controllers
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_driver
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_available_governors
sysctl kernel.numa_balancing
sysctl kernel.perf_event_paranoid
sysctl kernel.nmi_watchdog
swapon --show
```

`/sys/fs/cgroup/cgroup.controllers` 必须存在，否则不是 cgroup v2。改变 cgroup 模式需要修改启动配置并重启；此文档不授权 agent 自动修改。[Docker cgroup v2 文档](https://docs.docker.com/engine/containers/runmetrics/)，[Linux cgroup v2 文档](https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html)

## 3. 第二阶段：人工硬件与 BIOS 确认

以下项目通常必须通过 BMC/BIOS 人工确认，agent 只记录，不自动修改：

| 项目 | 要求 |
|---|---|
| 电源 profile | 三台使用 vendor 的 maximum/performance-oriented profile |
| Boost/CPB/Turbo | 开启，保留 CPU 产品的真实动态加速能力 |
| 人工功率上限 | 不设置低于该 SKU 额定配置的额外 power cap |
| SMT | BIOS 保持开启；主测试通过 cpuset 每物理核只选一个 thread，后续可跑 SMT-on |
| 深度 C-state | 三台采用统一策略；优先使用 performance profile 对应策略，不混用手动/默认配置 |
| NUMA/NPS | 保留并记录实际设置；不强求不同厂商使用不存在的同名选项 |
| Secure Boot/IOMMU | 状态一致并记录；不要为了 perf 随意关闭安全功能 |
| 固件 | 每个平台使用已批准的稳定 BIOS/BMC/microcode，然后在整个测试周期冻结 |

如果三台无法实现相同的策略目标，应在安装前回报差异。

## 4. 第三阶段：统一 OS 基础环境

本节假定三台已通过同一 Ubuntu 24.04 LTS 镜像安装，并且重装行为已另行获得授权。本文不包含磁盘分区和 OS 重装命令。

### 4.1 更新与工具包

在同一维护窗口，从相同 apt mirror 安装同一版本的软件。不要在三台上分散到不同日期执行无版本约束的 full-upgrade。

建议工具：

```text
ca-certificates
curl
gpg
jq
numactl
hwloc
sysstat
linux-tools-common
与 uname -r 匹配的 linux-tools 包
nvme-cli
ethtool
chrony
```

安装前先验证与当前 kernel 对应的 `linux-tools` 包确实存在。若不存在，不要安装另一个 kernel 的 perf；回报 kernel/tooling 不匹配。

不在测试机上安装桌面环境、Docker Desktop、Kubernetes、Podman、数据库或编译工具链。测试驱动和 probe 应在构建机上生成，再以完全相同的二进制和 image digest 分发。

### 4.2 时间同步

三台统一使用 chrony，并确认：

```bash
timedatectl status
chronyc -N tracking
```

必须看到系统时钟已同步。NTP 用于跨主机日志排序；单次 latency 仍应使用本机 monotonic clock。Ubuntu 提供 chrony 的状态和配置方法。[Ubuntu chrony 文档](https://ubuntu.com/server/docs/how-to/networking/chrony-client/)

### 4.3 CPU frequency policy

先记录实际 driver 和可用 governor。只有三台都确认支持后，才执行：

```bash
sudo cpupower frequency-set --governor performance
cpupower frequency-info
```

如果某个平台不支持 `performance` governor，不要写入未知 sysfs 节点或更换 CPU driver；回报 driver、available governors 和当前 policy，由协调者选择共同策略。

不要强制固定频率或关闭 boost。Linux 不同平台可能使用 `amd-pstate`、`amd-pstate-epp` 或 `acpi-cpufreq`；实际 driver 是平台特性，必须记录。[Linux CPUFreq 文档](https://docs.kernel.org/6.17/admin-guide/pm/cpufreq.html)，[AMD P-State 文档](https://docs.kernel.org/admin-guide/pm/amd-pstate.html)

### 4.4 不做持久化的“优化”

本阶段不自动执行以下操作：

- `isolcpus`/`nohz_full`/`rcu_nocbs` kernel boot 参数；
- 禁用 irqbalance；
- `mitigations=off`；
- 全局 drop page cache；
- 禁用 NMI watchdog；
- 修改 NUMA balancing、THP 或 swappiness；
- 固定 CPU frequency；
- 将 Docker 数据放入 tmpfs。

这些改动会改变被测系统含义或需要重启。主测试先采用同一发行版默认值加明确的 CPU/NUMA pinning；诊断性实验另行定义。

## 5. 第四阶段：安装并锁定 Docker

### 5.1 只允许 fresh install

Docker 官方说明发行版自带的 `docker.io`、`containerd` 和 `runc` 可能与官方 Docker CE 包冲突。只有在第二章已经确认机器无容器数据、且所有者允许的 fresh host 上，才可处理冲突包。[Docker Ubuntu 安装文档](https://docs.docker.com/engine/install/ubuntu/)

如果不是 fresh host，停止。不要根据通用卸载命令猜测哪些数据可以删除。

### 5.2 配置 Docker 官方 apt 仓库

使用 Docker 官方 GPG key 和 apt repository。执行前检查 `/etc/apt/keyrings/docker.asc` 和 `/etc/apt/sources.list.d/docker.sources` 是否已经存在；若存在，先核验，不覆盖。

官方流程如下：

```bash
sudo apt-get update
sudo apt-get install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
```

在冻结的 Ubuntu 24.04 `noble`/`amd64` 基线上，`/etc/apt/sources.list.d/docker.sources` 内容应为：

```text
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: noble
Components: stable
Architectures: amd64
Signed-By: /etc/apt/keyrings/docker.asc
```

如果 `/etc/os-release` 不是 `noble`，停止；不要机械写入该 source。source 文件写好后执行 `sudo apt-get update`。不使用 `get.docker.com` convenience script。

### 5.3 发现三台共同版本

添加仓库并更新索引后，先只查询：

```bash
apt-cache madison docker-ce
apt-cache madison docker-ce-cli
apt-cache madison containerd.io
```

把三台输出交给协调者，选择三台都存在的同一精确版本，填入第 1.2 节版本锁。不要各自选择列表第一项。

### 5.4 按精确版本安装

版本锁填好后，将以下占位符替换为冻结表中的完整 apt version，再使用精确版本安装：

```bash
sudo apt-get install \
  docker-ce='<LOCKED_DOCKER_CE_VERSION>' \
  docker-ce-cli='<LOCKED_DOCKER_CLI_VERSION>' \
  containerd.io='<LOCKED_CONTAINERD_IO_VERSION>'
```

不要原样复制 `<LOCKED_...>`；agent 必须用冻结表里的实际字符串替换。Buildx 和 Compose 不属于测试依赖，首版不安装。

安装成功后锁定包，避免测试过程中漂移：

```bash
sudo apt-mark hold docker-ce docker-ce-cli containerd.io
```

安全补丁升级应在测试 campaign 结束后统一进行；不要在长期生产环境无限期冻结。

### 5.5 统一 daemon 配置

对于 fresh benchmark host，`/etc/docker/daemon.json` 使用以下最小配置：

```json
{
  "exec-opts": [
    "native.cgroupdriver=systemd"
  ],
  "features": {
    "containerd-snapshotter": true
  },
  "log-driver": "local",
  "log-opts": {
    "max-file": "3",
    "max-size": "10m"
  }
}
```

注意：

- 如果该文件已存在，先停止并回报；不要覆盖或合并未知配置。
- benchmark 容器仍会在每次 API create 时显式指定 `log-driver=none`；daemon 的 `local` 只是非 benchmark 容器的安全默认值。
- 不配置 TCP daemon socket，只使用 `/var/run/docker.sock`。
- 不启用 rootless、user namespace remapping、experimental firewall backend 或自定义 runtime。
- 不在 daemon 级别关闭 bridge；CPU-focused 测试逐容器指定 `network=none`，之后仍需测试统一的默认 bridge。

写入后先校验，再重启：

```bash
sudo dockerd --validate --config-file /etc/docker/daemon.json
sudo systemctl enable docker.service containerd.service
sudo systemctl restart containerd.service
sudo systemctl restart docker.service
sudo systemctl status docker.service --no-pager
```

`dockerd --validate` 是 Docker 官方提供的配置验证入口。[dockerd reference](https://docs.docker.com/reference/cli/dockerd/)

Docker Engine 29+ fresh install 默认使用 containerd image store，显式开启该 feature 可避免新装/升级历史造成不一致；默认 snapshotter 为 overlayfs。[Docker containerd image store 文档](https://docs.docker.com/engine/storage/containerd/)

### 5.6 benchmark 用户

推荐使用专用账户 `dockerbench`。如果账户不存在，可在获得账户创建许可后创建；如果已存在，先检查 UID、group 和 home ownership。

```bash
getent passwd dockerbench
getent group docker
sudo adduser --disabled-password --gecos 'Docker CPU benchmark account' dockerbench
sudo usermod -aG docker dockerbench
sudo -iu dockerbench docker version
```

`adduser` 只在确认账户不存在且已获许可时执行，不能对已有账户重复创建。

将账户加入 `docker` group 后必须重新登录才能生效。Docker 官方明确说明：`docker` group 等价于给予 root-level privileges，因此只能授予专用可信账户。[Docker Linux post-install 文档](https://docs.docker.com/engine/install/linux-postinstall)

计时路径中测试驱动直接访问 `/var/run/docker.sock`，不得在每次操作前启动 `sudo docker ...`。

## 6. 第五阶段：Docker 与 PMU 验收

### 6.1 Docker 软件栈

```bash
docker version
docker info
containerd --version
runc --version
docker info --format '{{.CgroupDriver}} {{.CgroupVersion}}'
docker info --format '{{json .DriverStatus}}'
docker info --format '{{json .SecurityOptions}}'
docker context show
```

必须满足：

- Client/Server Docker 版本与冻结表一致；
- containerd/runc 三台一致；
- `CgroupDriver=systemd`、`CgroupVersion=2`；
- DriverStatus 包含 `io.containerd.snapshotter.v1`；
- 没有 rootless；
- context 指向本机默认 Unix socket，而不是远程 daemon。

### 6.2 数据路径

```bash
sudo findmnt -T /var/lib/docker
sudo findmnt -T /var/lib/containerd
sudo stat -f -c '%T' /var/lib/docker
sudo stat -f -c '%T' /var/lib/containerd
df -hT /var/lib/docker /var/lib/containerd
```

确认两个路径都位于已批准的本地存储上，三台文件系统类型和 mount options 一致。

### 6.3 功能 smoke test

功能 smoke test 可以按照 Docker 官方方式运行一次 `hello-world`；这一步允许联网拉取，只发生在环境验收阶段，不计入正式测试。正式测试镜像之后必须离线加载、按 digest 固定，并使用 pull-never。

随后至少验证一次：

```bash
docker run --rm --network none --log-driver none hello-world
```

记录 exit code、Docker events 顺序和是否有残留容器。

### 6.4 PMU 与系统观测能力

```bash
perf --version
sudo perf stat -a -e cycles,instructions -- sleep 1
perf list
mpstat -P ALL 1 3
vmstat 1 3
iostat -x 1 3
```

`cycles` 和 `instructions` 不能显示 `<not supported>` 或持续 `<not counted>`。若 baseline PMU 失败，返回 `BLOCKED`；不要通过随机修改 paranoid、加载未知模块或关闭安全配置绕过。

厂商专用 PMU 事件只需保存 `perf list`，不作为环境准入时的跨厂商共同指标。

### 6.5 空闲状态与温度

在正式测试前至少观察：

- `uptime` 和 run queue；
- 每 CPU 利用率及 migrations；
- BMC 或 `sensors` 可见的温度、功率和 throttle 状态；
- NVMe utilization、await 和 queue depth；
- `/proc/interrupts` 中高频 IRQ 所在 CPU。

不要在装配阶段永久禁用 irqbalance。后续测试计划根据拓扑选择低噪声核心，并在 fixed-resource profile 中显式分配 harness、Docker control plane 和容器 worker CPU。

## 7. 测试资产部署约束

环境验收完成后，测试资产必须满足：

- 一个在构建机生成的静态 x86-64 probe，不使用 `-march=native`；
- 三台加载同一个 OCI/Docker image tar，image digest 完全一致；
- 一个相同 checksum 的测试驱动二进制；
- 驱动通过本机 Unix socket 直接调用 Docker Engine API；
- 正式测试禁止 pull、build、Compose 和远程 registry；
- CPU-focused profile 显式指定 `network=none`、`log-driver=none`、无 healthcheck；
- 不在环境安装阶段创建持久化 benchmark 容器、volume 或 network。

## 8. 每台机器需要交付的环境 Manifest

建议每台创建独立目录保存原始文本输出，但不得写入项目的 `sources/` 目录。Manifest 至少包含：

```text
hostnamectl.txt
os-release.txt
uname.txt
cmdline.txt
lscpu.txt
lscpu-extended.txt
numactl-hardware.txt
dmidecode-cpu-memory.txt
virtualization-check.txt
bios-bmc-microcode.txt
lsblk.txt
findmnt.txt
nvme-list.txt
docker-version.txt
docker-info.txt
containerd-version.txt
runc-version.txt
daemon-json.txt
package-versions.txt
package-holds.txt
cpufreq.txt
cgroup.txt
perf-sanity.txt
perf-list.txt
time-sync.txt
interrupts.txt
idle-mpstat-vmstat-iostat.txt
sha256sums.txt
```

所有 manifest 文件应带采集时间和 hostname。不要只粘贴截图；保留机器可解析的文本。

## 9. 最终准入清单

只有全部打勾才可返回 `READY`：

- [ ] 独占 bare metal，无 hypervisor/overcommit
- [ ] 三台 OS 镜像和 kernel build 一致
- [ ] BIOS 采用统一的 performance intent，boost 开启，无额外功率限制
- [ ] socket/core/thread/NUMA/memory topology 已记录
- [ ] Docker 数据位于批准的本地 NVMe；文件系统和 mount options 一致
- [ ] 无原有容器数据或冲突 runtime
- [ ] Docker CE、CLI、containerd、runc 精确版本一致并已冻结
- [ ] rootful Docker、本机 Unix socket、cgroup v2/systemd
- [ ] containerd image store + overlayfs snapshotter
- [ ] seccomp/AppArmor/mitigations 等安全状态一致
- [ ] 时间同步正常
- [ ] CPU governor/policy 已记录并采用共同策略
- [ ] baseline PMU `cycles,instructions` 可用
- [ ] 无持续后台负载、热降频或存储异常
- [ ] smoke test 成功且无残留容器
- [ ] 完整环境 manifest 已交付

## 10. Agent 回报格式

```markdown
## Environment setup result

- Host: `<hostname>`
- CPU: `<exact model>`
- Status: `READY` / `BLOCKED`
- Reboot required: `yes` / `no`
- Frozen OS/kernel: `<value>`
- Frozen Docker/containerd/runc: `<value>`
- Docker storage backend: `<value>`
- Docker data devices/filesystems: `<value>`
- cgroup: `<version and driver>`
- CPU frequency driver/governor: `<value>`
- PMU sanity: `pass` / `fail`
- Manifest path: `<absolute path>`
- Blockers or deviations: `<none or exact details>`
```

如果状态为 `BLOCKED`，必须给出原始失败命令、完整错误和需要谁批准/处理，不能自行扩大变更范围。
