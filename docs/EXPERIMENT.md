# O4 监控与采集流程

迁移来源：Lab2提交 `2c133c68828c551c80ed2152dabd2691a5eb6902`。
`cmd/loadgen/`、`cmd/promsteps/`、`scripts/rates.sh`、`scripts/table.sh`、工具下载方式来自该版本。
压测器新增 `-body-file`，默认SLO改为20 ms；到达过程、统计方法和停止条件保留。
采集流程按新项目的 `/version`、独立端口、固定请求和CPU目标调整；没有向业务加入空转计算。

## 环境与端口

正式测量使用原云服务器；不需要第二台主机。本地WSL仅用于功能联调和展示。
需要Linux、Go 1.26+、make、Python3、curl、procps、sysstat、taskset。
procps提供vmstat；sysstat提供mpstat、iostat、pidstat。若缺工具，先补齐再测量。

| 组件 | 地址 | 正式测量绑核 |
|---|---|---|
| textstats | 127.0.0.1:18080 | CPU 1 |
| 新Prometheus | 127.0.0.1:19090 | CPU 0 |
| node_exporter | 127.0.0.1:9100 | 复用已运行的采集器；记录其绑核 |
| loadgen和本轮资源采集器 | 本机调用 | CPU 0 |

相同主机仍有共享内存、内核及可能的超线程干扰，因此保留压测器CPU和发送滞后指标。
CPU 1指逻辑核；`GOMAXPROCS=1`限制同时执行Go代码的数量，不等于服务总共只有一个线程。
原Lab2业务及其他压测是否同时运行，将在服务器部署阶段检查，避免与本轮争用。

## 构建与监控启动

以下为运行说明，正式部署由阶段审批后执行。
在新项目根目录构建三个二进制，已运行的服务需在重新构建前停止：

~~~bash
make fmt-check vet test build
scripts/get-tools.sh
tools/prometheus-3.13.4.linux-amd64/promtool check config deploy/prometheus.yml
~~~

下载脚本固定版本并验证官方SHA256；工具及其运行数据不提交Git。
现有9100可用时复用node_exporter，不重复启动；若没有则启动新采集器。
每个长期服务使用独立终端，或在部署阶段安排后台日志：

~~~bash
# 仅当9100没有现有node_exporter时运行
taskset -c 0 tools/node_exporter --web.listen-address=127.0.0.1:9100

# 新项目独立Prometheus，不修改原Lab2配置
taskset -c 0 tools/prometheus --config.file=deploy/prometheus.yml \
  --storage.tsdb.path=/dev/shm/o4-prom --web.listen-address=127.0.0.1:19090

# 新业务服务
taskset -c 1 bin/textstats -addr 127.0.0.1:18080
~~~

等待两个抓取目标均up。19090的存储路径必须与其他Prometheus不同。
`/dev/shm`数据会随主机重启丢失；每轮结束应立即保存CSV和原始日志。

## 固定输入、预测与正式测量

先生成固定输入，此命令遇到已有文件会停止：

~~~bash
python3 scripts/make-input.py data/request.json --text-bytes 65536
~~~

默认文本为65536字节，由1024种固定词循环生成。业务会实际解析、计数并排序。
正式测量前固定文本规模、内容及算法，先做低负载基线、记录每请求CPU成本和上限预测。
输入规模是否适合压出CPU上限，要用服务器基线验证；不要根据本地联调确定正式到达率。

以下 `RATES` 由后续预测阶段给出，不能把示意值当成实测上限：

~~~bash
export GEN_CPUS=0 SLO=0.02
# RATES由服务器基线与测前预测确定
scripts/run.sh o4-cpu-20ms data/request.json "$RATES"
~~~

默认每阶段30秒，Poisson到达、固定种子1；客户端超时2秒，SLO始终20 ms。
沿用Lab2 `-stop-below=0.5`：成功返回不足50%时提前停止后续阶段，并保留已测数据。
`max-inflight=20000`是客户端保护上限；若出现dropped，不能据此认定服务CPU达到上限。
客户端吞吐按成功数/完整阶段时长，服务层按阶段末尾25秒的Prometheus窗口，数值可能略有差异。
客户端p99含发送滞后和网络；服务端p99是直方图估计，两者不能要求完全相同。

默认结果放在相邻Lab2项目的 `runs/<实验名>/`。
服务和脚本必须在同一主机、服务从当前项目根目录启动；采集器拒绝覆盖同名目录。
正式运行要求新项目工作区干净且服务构建提交号等于HEAD；修改代码后先提交、停服、重建再测量。
`SVC`和`PROM`可指定其他本机端口；`OUT_ROOT`可改变结果目录。

## 结果文件与复查

| 文件 | 用途 |
|---|---|
| meta.txt、run.json | 实验类型、时间、主机、绑核、提交号、命令、请求和二进制SHA256 |
| request.json、sample-response.json | 本轮实际输入和测量前业务响应 |
| steps.csv、loadgen.log | 客户端吞吐、错误、延迟、SLI、压测器CPU、发送滞后及警告 |
| red.csv | 服务RED、CPU、在途及调度等待，见queries-red.txt |
| use.csv | 绑定核利用率、主机CPU、PSI和内存，见queries-use.txt |
| vmstat.log、mpstat.log、iostat.log、pidstat.log | 每秒资源原始记录；iostat覆盖所有磁盘，作为排查资料 |
| metrics-start.txt、metrics-end.txt | 运行前后服务指标原文 |
| queries-red.txt、queries-use.txt | 本轮实际查询快照 |

重新生成派生CSV需要Prometheus仍保留对应时间段的数据；使用归档查询和原阶段时间，不重新压测：

~~~bash
python3 scripts/requery.py /root/ops-lab2-9/runs/o4-cpu-20ms
scripts/table.sh /root/ops-lab2-9/runs/o4-cpu-20ms \
  ok_rps error_rate p99_ms slo_good server_rps server_error_rate server_p99_ms \
  cpu_cores cpu_ms_per_req svc_cpu_busy cpu_wait_threads inflight_avg
~~~

5xx错误率与客户端错误率分开记录；不存在的PSI等指标保留空值，无请求时比值可为NaN。
不要把缺失指标写成0。CPU上限需要同时观察吞吐平台、利用率、等待/积压及压测器状态。
最高一个已测负载未饱和时，只能说明能力至少达到该负载。
正式原始数据、资源账和最终结论归档Lab2仓库，并链接本目标项目的固定提交。

## 本地验证

先执行 `make fmt-check vet test race build` 和 `python3 -m unittest discover -s scripts -p 'test_*.py'`。
运行实际Prometheus/node_exporter与业务服务，再以独立临时目录和低负载运行：

~~~bash
RUN_KIND=integration OUT_ROOT=/tmp/o4-integration-runs \
  GEN_CPUS=0 scripts/run.sh local-chain data/request.json 20
~~~

本地绑核须换为当前可用的两个不同逻辑核；服务始终绑定其中一个核。
本地结果类型明确标为integration，不写入正式性能结论。
