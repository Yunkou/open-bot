import { useEffect, useState } from "react";
import { exchangeOIDCCode, setSession } from "../api";

export default function AuthCallback() {
  const [error, setError] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code") || "";
    const state = params.get("state") || "";
    if (!code) {
      setError("缺少授权码 code");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await exchangeOIDCCode(code, state);
        if (cancelled) return;
        setSession(res.token, res.user);
        window.location.replace("/");
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="brand auth-brand">
          <div className="logo">◈</div>
          <div>
            <div className="brand-title">open-bot</div>
            <div className="brand-sub">正在完成 Casdoor 登录…</div>
          </div>
        </div>
        {error ? (
          <>
            <div className="auth-error">{error}</div>
            <a className="primary" href="/" style={{ display: "inline-block", textAlign: "center", textDecoration: "none" }}>
              返回登录
            </a>
          </>
        ) : (
          <p className="muted">请稍候</p>
        )}
      </div>
    </div>
  );
}
