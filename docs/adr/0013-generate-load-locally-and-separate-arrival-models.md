---
status: accepted
---

# Generate load locally and separate arrival models

Capacity 与 Offered-load Runs 都由被测 host 上独占资源的 harness 通过本地 Docker Unix socket 生成，远程 coordinator 只能编排且不进入计时路径。constant-rate 是必测 headline arrival model，Poisson arrival 是独立 sensitivity result；driver 自身饱和会使 trial 无效。
