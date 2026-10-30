# RED 和 CPU 指标口径

迁移来源：Qiyao-25/ops-lab2-9 的提交
2c133c68828c551c80ed2152dabd2691a5eb6902。

- internal/metrics/metrics.go 及其测试原样迁入。
- process_linux.go 参考 cmd/enrolld/procstat.go，调整模块导入、Linux编译标记并增加可用性标记。
- telemetry.go 为文本统计接口新增采集包装器；保留阶段1业务算法。

## 业务统计范围

只观测路径 /api/analyze，包括成功POST、非法输入400、超限413和错误方法405。
/metrics、/healthz、/version及未匹配路径不计入业务请求。
method只允许标准HTTP方法标签，其他值归为OTHER；route不包含查询字符串、文本或用户输入。

| 名称 | 含义 |
|---|---|
| http_requests_total | 已完成业务请求的累计数，标签route、method、code |
| http_request_duration_seconds | 服务端处理耗时直方图，标签同上 |
| http_inflight_requests | 正在处理的业务请求数 |
| process_cpu_seconds_total | Linux进程用户态与内核态累计CPU秒数 |
| process_cpu_wait_seconds_total | 当前进程线程在内核运行队列中累计等待秒数 |
| go_sched_latencies_seconds | Go运行时调度等待直方图 |
| textstats_info | 构建版本、提交号和Go版本 |
| textstats_process_metrics_supported | Linux为1；其他平台为0，且不伪造CPU用量 |

RED的R为完成请求速率；E沿用Lab2服务端5xx/全部业务请求口径，4xx单独保留。
客户端仍需记录非2xx、超时与连接错误，不能用服务端5xx比例代替客户端错误率。

D从进入业务观测包装器到路由处理返回，不包含网络、计划发送滞后及进入处理器前的排队。
直方图可计算均值和插值得到的p99。异常退出而未完成响应的请求仍需由客户端错误统计补充。

20 ms达标比例是附加SLI，不替代RED。分子是code=200且耗时≤0.02秒的累计桶，
分母是全部业务请求数。桶边界包含0.02；请求超时与这个阈值分别配置。

进程CPU计数用于计算每请求CPU开销；指标抓取与运行时后台工作本身也消耗CPU，估计中应考虑这些开销。
内核等待计数按当前线程求和，线程退出可能影响累计值；应结合Go调度等待和节点CPU指标解释。
除以请求数的查询在无流量时可能得到NaN或Inf，无流量直方图p99也可能为NaN，不能解读为故障。

## 查询

deploy/queries-red.txt 提供R、E、D及附加SLI、CPU和等待查询。
占位符供promsteps使用；在Prometheus界面中应替换为实际值，例如：
route=/api/analyze、window=25s、slo=0.02。
所有查询限定job=textstats，避免混入enrolld指标。

已在阶段3使用实际Prometheus和node_exporter完成本地抓取、按阶段查询及CSV生成验证，
见 [VALIDATION.md](VALIDATION.md)。当前没有服务器性能结果，本地验证不作为正式压测。

## 本地检查

~~~bash
make fmt-check vet test build
make race
bin/textstats -addr 127.0.0.1:18080
~~~

另一个终端先发一次业务请求，再抓取指标：

~~~bash
curl -sS http://127.0.0.1:18080/api/analyze \
  -H 'Content-Type: application/json' -d '{"text":"Hello hello world"}'
curl -sS http://127.0.0.1:18080/metrics
~~~

Linux提供正式CPU实验所需的进程指标；其他平台仅适合功能演示，不能将缺失CPU指标解释为零消耗。
