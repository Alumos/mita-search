# 高小铁 AI 助手 · 复兴号知识库

**VPS / 1Panel 部署：** 使用 [`compose.yaml`](compose.yaml) 拉取完整镜像 `ghcr.io/alumos/mita-search:latest`，填写 `SILICONFLOW_API_KEY` 即可。详见 [部署说明](DEPLOY.md)。每次推送 `main`，GitHub Actions 自动构建并发布 amd64 / arm64 镜像。

一个面向小学信息技术课堂的 AI 搜索应用，**Go 后端 + Vue3/Element Plus 前端**，AI 能力接入 **SiliconFlow API**，检索范围锁定服务端内置的「复兴号」知识库。界面适配 1920×1080 课堂大屏。

聚焦教材第 3 课《系统自主可控的重要意义》最后一页 **"做一做"板块**：完成《"复兴号"动车组列车的自主创新及其优点》表格（自主建造 | 自主研发 | 优点）。

首页与搜索结果页底部均展示 [苏ICP备2021038338号-1](https://beian.miit.gov.cn/)，点击可在新标签页打开备案网站。

结果页在宽屏下并排显示回答与参考来源；窄屏下使用纵向滚动，回答卡片保留足够高度，避免嵌入较小窗口时被参考资料压缩。

## 功能特性

- 🚄 **"高小铁"品牌界面**：首页大搜索框、简洁/深入/研究三种模式、结果页流式讲解与动态成果表、右侧参考来源侧栏、"接着问"推荐、回答内 [1][2] 引用角标；全局大字号排版，适配 1920×1080
- 🧠 **RAG 检索增强**：内置 39 条课本要求与复兴号知识素材（报道来源：新华社、人民日报、中新社、央视网等），记录报道和核验日期；关键词检索优先排序，并提供既有事实资料供多角度归纳，约束模型依据资料作答
- ⚡ **流式输出 + 稳定性保障**：SSE 逐字生成；Qwen3 系思考模型自动 `enable_thinking: false` 关闭思考链；**45 秒首字看门狗**，超时自动切换备用模型
- 📋 **动态补充答案**：学生从自然搜索关键词出发，了解智能高铁关键核心技术自主创新的阶段性成果。模型在同一张三列表格中保留题目给定的两行，并自由归纳补充实例；不限制为固定的3/4/5行，不使用预制答案组合。
- 🧩 **单表收尾**：讲解流式显示，表格完整行逐行出现，并合并意外重复的表格与相同数据行，最终校验后保留一张。缺少补充答案或格式不完整时尝试备用模型，不追加固定答案。
- ✂️ **输出额度**：简洁/深入/研究分别使用1600/2800/4000 token上限，包含讲解和表格；模式控制详略，不决定固定行数。
- 🔒 **密钥安全**：API Key 仅存于服务端 `config.json`，前端不接触任何密钥
- 📦 **零构建依赖**：前端组件库（Vue3 / Element Plus / 图标库）已全部本地化到 `static/vendor/`，无需 Node 构建和外部 CDN

## 目录结构

```
mita-search/
├── main.go                  # Go 后端（静态服务 + 检索 + SSE 流式 RAG）
├── task_table.go            # 素材校验、资料检索、模型表格校验与去重
├── task_table_test.go       # 表格完整性、来源与流式接口测试
├── go.mod
├── config.json              # 端口 / SiliconFlow 密钥与模型配置（勿公开！）
├── stress_test.py           # 并发压测脚本（模拟 N 台学生机同时提问）
├── knowledge/
│   ├── SOURCES.md           # 联网核验来源与数据使用边界
│   └── knowledge_base.json  # 复兴号知识库（39 条，含课文要求与结构化填表素材）
└── static/
    ├── index.html           # 单页前端
    ├── css/style.css
    ├── js/app.js            # 搜索 / Markdown 渲染 / 流式状态管理
    └── vendor/              # 本地化的 Vue3 + Element Plus
```

## 本地运行

```bash
cd mita-search
go build -o mita-search .
./mita-search
# 打开 http://localhost:8066
```

首次从 GitHub 下载运行时，先复制 `config.example.json` 为 `config.json` 并填写密钥，或通过环境变量 `SILICONFLOW_API_KEY` 设置密钥。环境变量优先于配置文件。

## 配置说明（config.json）

```json
{
  "listen_port": 8066,
  "siliconflow": {
    "api_key": "sk-****",
    "base_url": "https://api.siliconflow.cn/v1/chat/completions",
    "model": "Qwen/Qwen3.5-27B",
    "fallback_models": ["Qwen/Qwen2.5-72B-Instruct", "Qwen/Qwen2.5-7B-Instruct"],
    "max_tokens": 3000,
    "temperature": 0.5
  }
}
```

> ⚠️ `config.json` 含真实 API Key，请勿提交到公开仓库。`max_tokens` 为兜底值，实际按模式动态限制（见上）。换模型直接改 `model` 字段；模型名含 `Qwen3` 时会自动关闭思考链。

## 50 台学生机并发压测

```bash
python3 stress_test.py 50   # 可自定义并发数
```

原版本历史实测报告（Qwen3.5-27B，50 并发；不代表本次知识库和任务表改版后的性能）：

| 指标 | 结果 |
|------|------|
| 成功率 | **50/50（100%）** |
| 整批完成 | 28.6s |
| 首字延迟 | 平均 2.2s / P50 2.3s / 最大 3.7s |
| 单请求完成 | 平均 16.3s / 最大 28.6s |
| 回答字数 | 平均 402 字（限额内） |
| 模型分布 | 全部命中 Qwen/Qwen3.5-27B，无降级 |

> 压测脚本使用标准库实现，逐行解析 SSE 统计首字延迟与回答长度，可直接在教师机运行。

## 服务器部署（nginx 反向代理示例）

推荐使用 [Docker Compose / 1Panel 部署](DEPLOY.md)，以下为非容器部署方式。

1. 服务器上安装 Go ≥ 1.21，上传本项目，`go build -o mita-search .` 后运行（建议 systemd 守护）；
2. nginx 反向代理示例（注意关闭缓冲以支持 SSE 流式）：

```nginx
location / {
    proxy_pass http://127.0.0.1:8066;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;          # SSE 必须
    proxy_cache off;
    proxy_read_timeout 300s;
}
```

## API 一览

| 接口 | 方法 | 说明 |
|------|------|------|
| `/api/search` | POST | `{query, mode}` → SSE 流：`meta` / `references` / `ping` / `delta` / `answer` / `retry` / `done` / `error` |
| `/api/kb` | GET | 知识库元信息（名称、条数、分类） |
| `/api/health` | GET | 健康检查 |

## 知识库维护

编辑 `knowledge/knowledge_base.json` 的 `items`；每条包含 `id / title / category / content / keywords / source / url / published_at / verified_at`。课文要求单独标记为用户提供的教材内容，不伪装成网络报道。原文核验清单见 [knowledge/SOURCES.md](knowledge/SOURCES.md)。

知识库文件及其 `task_columns` 保留原样；其中的预制表格素材不再作为答案拼接或注入模型。检索排除教学说明条目，将匹配内容优先排列，并补齐已有示例和自动驾驶阶段所需资料，供模型从不同技术、阶段和应用角度归纳。

模型独立生成讲解和补充表格。后端校验原有示例、补充行、三列结构和引用编号，合并意外重复的表格及相同数据行。`delta` 事件传输讲解，`answer` 事件更新同一张逐行展开的表格及最终答案，随后发送 `done`。多样性来自资料选取、具体成果和表述，不用随机编造或堆砌重复内容凑行数。

## 验证

```bash
go test -race ./...
go vet ./...
node --test tests/frontend.test.cjs
```

Go 测试使用本地模拟上游，不需要真实 API Key；前端回归测试只需 Node.js 18+，无额外依赖。实际模型的讲解效果需配置 SiliconFlow API Key 后验证。
