# 1Panel / Docker Compose 部署

项目只有一个服务：Go 后端、Vue/Element Plus 前端、本地前端组件和完整知识库都在 `ghcr.io/alumos/mita-search:latest` 镜像内。无需在 VPS 安装 Go、Node.js 或数据库，也无需上传源代码或挂载配置文件。支持 Linux amd64 和 arm64。

## 1. 首次镜像发布

推送到 `main` 后，GitHub Actions 会执行检查、构建两个架构的镜像、发布到 GHCR，再验证已发布镜像的健康接口、知识库及前端文件。

- 构建状态：https://github.com/Alumos/mita-search/actions
- 镜像页面：https://github.com/users/Alumos/packages/container/package/mita-search
- 镜像标签：`latest`、`sha-提交前7位`；推送 `v1.0.0` 等标签时还会发布版本标签。

**GHCR 首次创建的镜像包可能是 Private。** 在镜像页面的 **Package settings → Change visibility → Public** 设置为公开后，VPS 可以直接拉取。源码仓库公开不代表镜像包已公开。

如果保留私有镜像，在 VPS 上使用具有 `read:packages` 权限的 GitHub token 登录 GHCR，或在 1Panel 的镜像仓库中配置 `ghcr.io` 和 GitHub 用户名/token：

```bash
docker login ghcr.io -u Alumos
```

## 2. 在 1Panel 创建编排

1. 打开 **容器 → 编排 → 创建编排**，名称填 `mita-search`。
2. 将仓库中的 `compose.yaml` 全部粘贴到 Compose 内容中。
3. 在编排环境变量中填写 `SILICONFLOW_API_KEY=你的真实SiliconFlow密钥`，可选 `MITA_PORT=8066`。如面板版本没有环境变量入口，将 YAML 中的 `SILICONFLOW_API_KEY` 值改成 `"你的真实SiliconFlow密钥"`；端口默认 8066。
4. 启动编排，等待容器健康状态变为 `healthy`。
5. 访问 `http://VPS公网IP:8066`；使用直接访问时，在 VPS 防火墙与云厂商安全组开放对应 TCP 端口。

密钥在 VPS 运行时注入。镜像和 GitHub 仓库不包含本地 `config.json` 的真实密钥。

## 3. 可选：1Panel 网站反向代理

在 1Panel 网站中创建反向代理，目标为 `http://127.0.0.1:8066`。如果 OpenResty 运行在另一个容器中，`127.0.0.1` 指向该容器本身，需要使用能访问宿主机的地址，或让两个容器加入同一个网络并使用 `http://mita-search:8066`。

在对应代理 location 中保留面板原有配置，并加入以下设置，保证 AI 回答逐字显示：

```nginx
proxy_http_version 1.1;
proxy_set_header Connection "";
proxy_buffering off;
proxy_cache off;
proxy_read_timeout 300s;
```

## 命令行部署与更新

将 `compose.yaml` 和 `.env.example` 下载至 VPS 的同一目录：

```bash
cp .env.example .env
# 编辑 .env，填写 SILICONFLOW_API_KEY
docker compose up -d
docker compose ps
curl -fsS http://127.0.0.1:8066/api/health
```

更新后，在 1Panel 重新拉取镜像并重建编排；或执行：

```bash
docker compose pull
docker compose up -d
```

如需固定版本，把 `image` 中的 `latest` 改成成功发布的 `sha-xxxxxxx` 或版本号。

## 可选配置

| 环境变量 | 默认值 / 含义 |
| --- | --- |
| `SILICONFLOW_API_KEY` | 必填；SiliconFlow API 密钥 |
| `MITA_PORT` | `8066`；宿主机对外端口 |
| `SILICONFLOW_BASE_URL` | `https://api.siliconflow.cn/v1/chat/completions` |
| `SILICONFLOW_MODEL` | `Qwen/Qwen3.5-27B` |
| `SILICONFLOW_FALLBACK_MODELS` | `Qwen/Qwen2.5-72B-Instruct,Qwen/Qwen2.5-7B-Instruct`；设置为空可关闭备用模型 |

容器内部固定使用 8066 端口。知识库更新应修改源码中的 `knowledge/knowledge_base.json` 并推送，等待新镜像发布后更新编排。健康接口只验证服务存活，不会调用收费的 AI API。
