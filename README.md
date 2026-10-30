# textstats：Lab2 O4 的独立目标项目

提供 HTTP 文本统计服务，作为迁入 Lab2 RED 指标、Prometheus 和资源测量方法的目标应用。

当前为阶段1：业务接口、构建入口和测试。RED 接入、服务器部署、测前预测及正式测量将在后续阶段完成；当前没有声称 O4 已完成。

## 业务规则

输入文本按连续的 Unicode 字母或数字分词，统一为小写；统计总词数、不同词数和前10个高频词。先按次数降序排列，同频按词语字典序排列。输出稳定，便于固定请求进行可重复测量。

- POST /api/analyze：JSON 请求体只有 text 字段；空白文本返回400。
- 请求体上限1 MiB；超限返回413；非法JSON、未知字段或多个JSON值返回400。
- GET /healthz：返回 ok。
- GET /version：返回构建版本、提交号、Go版本、PID和GOMAXPROCS。

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

## O4 后续测量约定

- 统一使用20 ms（SLO=0.02）作为达标阈值；请求超时另行配置。
- 计划测CPU资源上限；文本解析、词频统计和排序均为实际业务工作。
- 正式实验固定输入内容、规模和输出规则，先记录预测，再加压。
- 现有云服务器上服务使用逻辑CPU 1，负载发生器和监控使用CPU 0。
- 不将最高已测速率自动称为上限；检查CPU饱和、吞吐平台、等待和压测器状态。
- 项目代码在本仓库维护，正式原始数据与资源账在Lab2仓库归档，记录两边提交号。

迁移方法来源：Qiyao-25/ops-lab2-9 的 main 提交
2c133c68828c551c80ed2152dabd2691a5eb6902（2026-10-30读取的最新版本）。
阶段2将参考其 internal/metrics/ 接入指标；不需要第二台主机。
