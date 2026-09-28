import { Button, Card, Result, Typography } from "antd";
import { WEB_URL } from "../api";
import { useAuth } from "../auth/AuthContext";

const { Link, Text } = Typography;

export default function DeniedPage() {
  const { user, denied, logout } = useAuth();

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
      <Card style={{ width: "min(520px, 100%)" }}>
        <Result
          status="403"
          title="无管理端权限"
          subTitle={
            <>
              {denied || "此账号无管理端权限"}。普通用户请使用用户端{" "}
              <Link href={WEB_URL}>{WEB_URL}</Link>。
            </>
          }
          extra={[
            <Button key="web" href={WEB_URL}>
              打开用户聊天端
            </Button>,
            <Button key="clear" type="primary" onClick={logout}>
              清除会话并返回登录
            </Button>,
          ]}
        />
        <Text type="secondary">
          当前角色：{user?.role || "—"} · {user?.username || "—"}
        </Text>
      </Card>
    </div>
  );
}
