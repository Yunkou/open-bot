import { useEffect, useState } from "react";
import { Alert, Button, Card, Divider, Form, Input, Typography } from "antd";
import {
  WEB_URL,
  clearSession,
  exchangeOIDCCode,
  fetchOIDCConfig,
  login,
  setSession,
  startOIDCLogin,
} from "../api";
import { useAuth } from "../auth/AuthContext";

const { Title, Paragraph, Link } = Typography;

export default function LoginPage() {
  const { setUser } = useAuth();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [oidcEnabled, setOidcEnabled] = useState(false);
  const [form] = Form.useForm();

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
        <Paragraph type="secondary" style={{ marginTop: 16, marginBottom: 0, fontSize: 13 }}>
          <Link href={WEB_URL}>← 打开用户聊天端</Link>
        </Paragraph>
      </Card>
    </div>
  );
}
