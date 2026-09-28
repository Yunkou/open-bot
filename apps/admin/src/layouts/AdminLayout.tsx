import { useMemo } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { Dropdown, Space, Typography } from "antd";
import {
  AuditOutlined,
  BarChartOutlined,
  ClusterOutlined,
  DeploymentUnitOutlined,
  FlagOutlined,
  LogoutOutlined,
  NodeIndexOutlined,
  TeamOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { ProLayout } from "@ant-design/pro-components";
import { WEB_URL } from "../api";
import { useAuth } from "../auth/AuthContext";

const { Text, Link: ALink } = Typography;

const menuRoutes = {
  path: "/",
  routes: [
    {
      path: "/users",
      name: "用户管理",
      icon: <UserOutlined />,
    },
    {
      path: "/bots",
      name: "Bot 管理",
      icon: <DeploymentUnitOutlined />,
    },
    {
      path: "/traces",
      name: "调用追踪",
      icon: <NodeIndexOutlined />,
    },
    {
      path: "/members",
      name: "成员与角色",
      icon: <TeamOutlined />,
    },
    {
      path: "/llm",
      name: "默认模型",
      icon: <ClusterOutlined />,
    },
    {
      path: "/usage",
      name: "用量",
      icon: <BarChartOutlined />,
    },
    {
      path: "/flags",
      name: "功能开关",
      icon: <FlagOutlined />,
    },
    {
      path: "/audit",
      name: "审计日志",
      icon: <AuditOutlined />,
    },
  ],
};

export default function AdminLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, orgName, logout } = useAuth();

  const roleLabel = useMemo(() => {
    if (user?.role === "platform_admin") return "平台管理员";
    if (user?.role === "org_admin") return "组织管理员";
    return user?.role || "";
  }, [user?.role]);

  return (
    <div style={{ height: "100vh" }}>
      <ProLayout
        title="open-bot 管理端"
        logo={false}
        layout="mix"
        fixSiderbar
        fixedHeader
        location={{ pathname: location.pathname }}
        route={menuRoutes}
        menuItemRender={(item, dom) =>
          item.path ? <Link to={item.path}>{dom}</Link> : dom
        }
        avatarProps={{
          icon: <UserOutlined />,
          title: user?.username || "管理员",
          size: "small",
          render: (_props, dom) => (
            <Dropdown
              menu={{
                items: [
                  {
                    key: "logout",
                    icon: <LogoutOutlined />,
                    label: "退出登录",
                    onClick: () => {
                      logout();
                      navigate("/login", { replace: true });
                    },
                  },
                ],
              }}
            >
              {dom}
            </Dropdown>
          ),
        }}
        actionsRender={() => [
          <Space key="org" size={4} style={{ marginRight: 8 }}>
            <Text type="secondary">{orgName || "组织"}</Text>
            <Text type="secondary">·</Text>
            <Text type="secondary">{roleLabel}</Text>
          </Space>,
        ]}
        headerContentRender={() => (
          <ALink href={WEB_URL} style={{ marginLeft: 16, fontSize: 13 }}>
            ← 用户聊天端
          </ALink>
        )}
      >
        <Outlet />
      </ProLayout>
    </div>
  );
}
