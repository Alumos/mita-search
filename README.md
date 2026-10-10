# 高小铁 AI 助手 · 复兴号知识库

**VPS / 1Panel 部署：** 使用 [`compose.yaml`](compose.yaml) 拉取完整镜像 `ghcr.io/alumos/mita-search:latest`，填写 `SILICONFLOW_API_KEY` 即可。详见 [部署说明](DEPLOY.md)。每次推送 `main`，GitHub Actions 自动构建并发布 amd64 / arm64 镜像。

一个面向小学信息技术课堂的 AI 搜索应用，**Go 后端 + Vue3/Element Plus 前端**，AI 能力接入 **SiliconFlow API**，检索范围锁定服务端内置的「复兴号」知识库。界面适配 1920×1080 课堂大屏。

聚焦教材第 3 课《系统自主可控的重要意义》最后一页 **"做一做"板块**：完成《"复兴号"动车组列车的自主创新及其优点》表格（自主建造 | 自主研发 | 优点）。

首页与搜索结果页底部均展示 [苏ICP备2021038338号-1](https://beian.miit.gov.cn/)，点击可在新标签页打开备案网站。

结果页在宽屏下并排显示回答与参考来源；窄屏下使用纵向滚动，回答卡片保留足够高度，避免嵌入较小窗口时被参考资料压缩。

## 功能特性

- 🚄 **"高小铁"品牌界面**：首页大搜索框、简洁/深入/研究三种模式、结果页流式讲解与完整任务表、右侧参考来源侧栏、"接着问"推荐、回答内 [1][2] 引用角标；全局大字号排版，适配 1920×1080
- 🧠 **RAG 检索增强**：内置 39 条课本要求与复兴号知识素材（报道来源：新华社、人民日报、中新社、央视网等），记录报道和核验日期；关键词检索后补齐任务表必需来源，约束模型依据资料作答
- ⚡ **流式输出 + 稳定性保障**：SSE 逐字生成；Qwen3 系思考模型自动 `enable_thinking: false` 关闭思考链；**45 秒首字看门狗**，超时自动切换备用模型
- 📋 **完整任务表**：每次成功回答都以“自主建造｜自主研发｜优点”三列表格收尾，保留课本六个示例；简洁/深入/研究分别提供 3/4/5 行，扩展项随问题重点调整。表格由已核验素材生成，附来源编号，不依赖模型遵守表格格式。
- ✂️ **讲解长度控制**：简洁约120–180字（600 tok）、深入约250–400字（1100 tok）、研究约450–650字（1800 tok）；任务表另行追加，不占模型输出额度。
- 🔒 **密钥安全**：API Key 仅存于服务端 `config.json`，前端不接触任何密钥
- 📦 **零构建依赖**：前端组件库（Vue3 / Element Plus / 图标库）已全部本地化到 `static/vendor/`，无需 Node 构建和外部 CDN

## 目录结构

```
mita-search/
├── main.go                  # Go 后端（静态服务 + 检索 + SSE 流式 RAG）
├── task_table.go            # 任务素材校验、按问题选择扩展项、生成末尾表格
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
| `/api/search` | POST | `{query, mode}` → SSE 流：`meta` / `references` / `delta` / `retry` / `done` / `error` |
| `/api/kb` | GET | 知识库元信息（名称、条数、分类） |
| `/api/health` | GET | 健康检查 |

## 知识库维护

编辑 `knowledge/knowledge_base.json` 的 `items`；每条包含 `id / title / category / content / keywords / source / url / published_at / verified_at`。课文要求单独标记为用户提供的教材内容，不伪装成网络报道。原文核验清单见 [knowledge/SOURCES.md](knowledge/SOURCES.md)。

`task_columns` 分别配置自主建造、自主研发、优点三栏素材，每栏前两项必须是课本示例，每栏至少5项。每项含 `text / detail / reference_ids / keywords`；引用须指向真实知识条目。启动时校验这些约束，缺失素材或无效引用会报错。选出的表格来源会补入模型上下文和页面来源列表。

三栏是按课本分类分别列举，不表示同行存在一一对应关系。“自主建造”与“自主研发”也不是互斥过程。数据使用须区分车型、报道时间、试验和运营；例如2018年京沈现场试验与2019年京张自动驾驶应用要分别表述。

## 验证

```bash
go test -race ./...
go vet ./...
node --test tests/frontend.test.cjs
```

Go 测试使用本地模拟上游，不需要真实 API Key；前端回归测试只需 Node.js 18+，无额外依赖。实际模型的讲解效果需配置 SiliconFlow API Key 后验证。
