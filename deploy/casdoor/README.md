# Casdoor（本地 OIDC）

Compose 已合并到仓库 **`deploy/compose.yaml`**（profile `casdoor`），不共用 open-bot Postgres。

```bash
make compose-casdoor
# UI: http://localhost:8000  （built-in/admin / 123）
# DB: 127.0.0.1:5434
# 停止：make compose-casdoor-down
```

配置目录：`conf/`（挂载到容器 `/conf`；库主机名为 `casdoor-postgres`）。

在 Casdoor **Applications → app-built-in**：

1. 记下 Client ID / Client secret
2. Redirect URLs 增加：`http://127.0.0.1:5173/auth/callback`、`http://localhost:5173/auth/callback`
3. 写入仓库根 `.env`（见 `.env.example` 的 `CASDOOR_*`）

然后重启 `make dev-api`；Web 登录页出现「用 Casdoor 登录」。

更多见 [deploy/README.md](../README.md) 与 [docs/本地启动.md](../../docs/本地启动.md)。
