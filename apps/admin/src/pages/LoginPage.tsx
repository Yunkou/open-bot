import { useEffect, useState } from "react";
import { Alert, Button, Card, Divider, Form, Input, Typography } from "antd";
import {
  DEFAULT_API_BASE,
  WEB_URL,
  clearSession,
  exchangeOIDCCode,
  fetchOIDCConfig,
  formatApiHealthProbe,
  getApiBase,
  login,
  probeApiHealth,
  setApiBase,
  setSession,
  startOIDCLogin,
  type ApiHealthProbeResult,
} from "../api";
import { useAuth } from "../auth/AuthContext";

const { Title, Paragraph, Link } = Typography;

export default function LoginPage() {
  const { setUser } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [form] = Form.useForm();
  const [apiDraft, setApiDraft] = useState(() => getApiBase());
  const [apiBusy, setApiBusy] = useState(false);
  const [apiProbe, setApiProbe] = useState<ApiHealthProbeResult | null>(null);

  useEffect(() => {
    void fetchOIDCConfig()
      .then((c) => setOidcEnabled(Boolean(c.enabled)))
      .catch(() => setOidcEnabled(false));
  }, []);

  // Handle Casdoor callback when redirect_uri points at admin (optional).
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const state = params.get("state");
    if (!code) return;
    setBusy(true);
    void exchangeOIDCCode(code, state || "")
      .then((res) => {
        setSession(res.token, res.user);
        window.history.replaceState({}, "", "/login");
        setUser(res.user);
      })
      .catch((err) => {
        clearSession();
        const msg = err instanceof Error ? err.message : String(err);
        if (msg.includes("无管理端权限")) {
          setError(`此账号无管理端权限。普通用户请打开用户端 ${WEB_URL}`);
        } else {
          setError(msg);
        }
      })
      .finally(() => setBusy(false));
  }, [setUser]);

  const onFinish = async (values: { username: string; password: string }) => {
    setBusy(true);
    setError("");
    try {
      const res = await login(values.username.trim(), values.password);
      setSession(res.token, res.user);
      setUser(res.user);
    } catch (err) {
      clearSession();
      const msg = err instanceof Error ? err.message : String(err);
      if (msg.includes("无管理端权限")) {
        setError(`此账号无管理端权限。普通用户请打开用户端 ${WEB_URL}`);
      } else {
        setError(msg);
      }
    } finally {
      setBusy(false);
    }
  };

  const onOidc = async () => {
    setBusy(true);
    setError("");
    try {
      const { authorize_url } = await startOIDCLogin();
      window.location.href = authorize_url;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  const onProbeApi = async () => {
    setApiBusy(true);
    setApiProbe(null);
    try {
      setApiProbe(await probeApiHealth(apiDraft));
    } finally {
      setApiBusy(false);
    }
  };

  const onApplyApi = async () => {
    setApiBusy(true);
    setApiProbe(null);
    try {
      const next = apiDraft.trim() || DEFAULT_API_BASE;
      const result = await probeApiHealth(next);
      setApiProbe(result);
      if (!result.ok) return;
      const applied = setApiBase(next);
      setApiDraft(applied);
    } finally {
      setApiBusy(false);
    }
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        padding: 24,
        background: "#f5f5f5",
      }}
    >
      <Card style={{ width: "min(400px, 100%)" }}>
        <Title level={3} style={{ marginTop: 0 }}>
          open-bot 管理端
        </Title>
        <Paragraph type="secondary" style={{ marginBottom: 16 }}>
          管理端仅供组织/平台管理员。普通用户请用用户端{" "}
          <Link href={WEB_URL}>{WEB_URL}</Link>。
        </Paragraph>
        <Form form={form} layout="vertical" onFinish={(v) => void onFinish(v)} requiredMark={false}>
          <Form.Item
            label="用户名"
            name="username"
            rules={[{ required: true, message: "请输入用户名" }]}
          >
            <Input autoFocus autoComplete="username" />
          </Form.Item>
          <Form.Item
            label="密码"
            name="password"
            rules={[{ required: true, message: "请输入密码" }]}
          >
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          {error ? (
            <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />
          ) : null}
          <Form.Item style={{ marginBottom: 0 }}>
            <Button type="primary" htmlType="submit" block loading={busy}>
              登录
            </Button>
          </Form.Item>
        </Form>
        {oidcEnabled ? (
          <>
            <Divider plain>或</Divider>
            <Button block disabled={busy} onClick={() => void onOidc()}>
              用 Casdoor 登录
            </Button>
            <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0, fontSize: 13 }}>
              Casdoor Redirect URI 需包含本管理端回调（若走 OIDC）。日常也可用密码登录。
            </Paragraph>
          </>
        ) : null}
        <Divider plain style={{ marginTop: 20, marginBottom: 12 }}>
          服务器
        </Divider>
        <Paragraph type="secondary" style={{ marginBottom: 8, fontSize: 13 }}>
          切换管理端对接的 API。先检测连接，通过后再登录；局域网或 ngrok 地址与用户端保持一致以便配对。
        </Paragraph>
        <Form.Item label="服务器地址" style={{ marginBottom: 8 }}>
          <Input
            value={apiDraft}
            placeholder={DEFAULT_API_BASE}
            disabled={apiBusy || busy}
            onChange={(e) => {
              setApiDraft(e.target.value);
              setApiProbe(null);
            }}
          />
        </Form.Item>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 8 }}>
          <Button disabled={apiBusy || busy} onClick={() => void onProbeApi()}>
            {apiBusy ? "检测中…" : "检测连接"}
          </Button>
          <Button type="primary" disabled={apiBusy || busy} onClick={() => void onApplyApi()}>
            使用此服务器
          </Button>
          {apiDraft.trim().replace(/\/$/, "") !== DEFAULT_API_BASE ? (
            <Button
              disabled={apiBusy || busy}
              onClick={() => {
                setApiDraft(DEFAULT_API_BASE);
                setApiProbe(null);
              }}
            >
              恢复默认
            </Button>
          ) : null}
        </div>
        {apiProbe ? (
          <Alert
            type={apiProbe.ok ? "success" : "error"}
            showIcon
            message={formatApiHealthProbe(apiProbe)}
            style={{ marginBottom: 12 }}
          />
        ) : (
          <Paragraph type="secondary" style={{ marginBottom: 12, fontSize: 12 }}>
            当前：{getApiBase()}
          </Paragraph>
        )}
        <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0, fontSize: 13 }}>
          <Link href={WEB_URL}>← 打开用户聊天端</Link>
        </Paragraph>
      </Card>
    </div>
  );
}
