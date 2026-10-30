# textstats：Lab2 O4 的独立目标项目

提供 HTTP 文本统计服务，作为迁入 Lab2 RED 指标、Prometheus 和资源测量方法的目标应用。

已完成业务接口、RED和Linux进程CPU指标接入，以及云服务器部署、测前预测和CPU饱和测量。
正式报告与原始数据在Lab2仓库的 [O4.md](https://github.com/Qiyao-25/ops-lab2-9/blob/codex/member4-o4/O4.md)。

## 业务规则

输入文本按连续的 Unicode 字母或数字分词，统一为小写；统计总词数、不同词数和前10个高频词。先按次数降序排列，同频按词语字典序排列。输出稳定，便于固定请求进行可重复测量。

- POST /api/analyze：JSON 请求体只有 text 字段；空白文本返回400。
- 请求体上限1 MiB；超限返回413；非法JSON、未知字段或多个JSON值返回400。
- GET /healthz：返回 ok。
- GET /version：返回构建版本、提交号、Go版本、PID和GOMAXPROCS。
- GET /metrics：Prometheus文本格式的RED、进程和Go运行时指标；抓取不计入业务流量。

## 构建与运行

需要 Go 1.26 或更高版本及 GNU make，无第三方Go依赖。

~~~bash
make fmt-check vet test build
bin/textstats -addr 127.0.0.1:18080
~~~

另一个终端：

~~~bash
curl -sS http://127.0.0.1:18080/api/analyze \
  -H 'Content-Type: application/json' \
  -d '{"text":"Hello hello world"}'
~~~

预期返回：

~~~json
{"word_count":3,"unique_words":2,"top_words":[{"word":"hello","count":2},{"word":"world","count":1}]}
~~~

按 Ctrl+C 或发送 SIGTERM 可优雅停止。bin/ 为本地构建产物，不纳入Git。

## 指标接入

已迁入Lab2指标库，并为 /api/analyze 加入请求计数、状态码分类、耗时直方图及在途数。
20 ms桶边界用于附加SLI，错误率按服务端5xx/全部业务请求计算；4xx与客户端错误分别解释。
Linux额外输出累计进程CPU时间及调度等待指标，供后续资源上限测量。

完整指标口径、迁移来源和本地检查方式见 [docs/METRICS.md](docs/METRICS.md)；
PromQL见 [deploy/queries-red.txt](deploy/queries-red.txt)。并发安全检查可运行 `make race`。

## 监控与实验采集

Prometheus 使用独立端口19090，避免与原Lab2的9090冲突；业务服务18080，node_exporter默认9100。
已迁入开放到达率压测器、按阶段查询工具、资源采集和Markdown表格脚本。
负载输入从固定JSON文件读取，避免大文本超过命令行长度限制。

安装工具、启动方式、采集命令与结果口径见 [docs/EXPERIMENT.md](docs/EXPERIMENT.md)。
`RUN_KIND=integration` 只用于本地验证，正式数据必须在云服务器测量。

## O4 测量约定与结果

- 统一使用20 ms（SLO=0.02）作为达标阈值；请求超时另行配置。
- 测量CPU资源上限；文本解析、词频统计和排序均为实际业务工作。
- 正式实验固定输入内容、规模和输出规则，先记录预测，再加压。
- 现有云服务器上服务使用逻辑CPU 1，负载发生器和监控使用CPU 0。
- 不将最高已测速率自动称为上限；检查CPU饱和、吞吐平台、等待和压测器状态。
- 项目代码在本仓库维护，正式原始数据与资源账在Lab2仓库归档，记录两边提交号。

本轮测量固定本仓库提交 `7503da5e1974a466f20ef7008bd20986b6aafdcb`，Go 1.26.9，
输入文本65536字节，单逻辑核，阶段30秒。最高已测零错误吞吐596.6 TPS；
650 RPS补测时，OS日志确认进程约占99.88%单核，客户端59.42%超时。
本配置约600 TPS量级进入饱和区，未精确定位硬极限或长期稳定容量。
按客户端p99≤20 ms且无错误，最高已测达标目标451 RPS、实际成功吞吐447.5 TPS。
补测的发送滞后告警与Prometheus CPU低估均在报告中保留并解释，不改写原始数据。

复现本轮时使用上述固定提交构建。若使用后续提交重新测量，应保存新的提交号和独立运行目录。

迁移方法来源：Qiyao-25/ops-lab2-9 的 main 提交
2c133c68828c551c80ed2152dabd2691a5eb6902（2026-10-30读取的最新版本）。
阶段2已参考其 internal/metrics/ 完成接入；不需要第二台主机。
