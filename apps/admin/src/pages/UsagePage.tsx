import { useEffect, useState } from "react";
import { Card, Col, Row, Statistic, message } from "antd";
import { PageContainer } from "@ant-design/pro-components";
import {
  CommentOutlined,
  MessageOutlined,
  RobotOutlined,
  TeamOutlined,
} from "@ant-design/icons";
import { adminGetUsage, type OrgUsage } from "../api";

export default function UsagePage() {
  const [usage, setUsage] = useState<OrgUsage | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    void adminGetUsage()
      .then(setUsage)
      .catch((err) => {
        message.error(err instanceof Error ? err.message : String(err));
      })
      .finally(() => setLoading(false));
  }, []);

  return (
    <PageContainer title="用量">
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="成员数"
              value={usage?.member_count ?? 0}
              prefix={<TeamOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="会话数"
              value={usage?.conversation_count ?? 0}
              prefix={<CommentOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="消息数"
              value={usage?.message_count ?? 0}
              prefix={<MessageOutlined />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={6}>
          <Card loading={loading}>
            <Statistic
              title="助手数"
              value={usage?.agent_count ?? 0}
              prefix={<RobotOutlined />}
            />
          </Card>
        </Col>
      </Row>
    </PageContainer>
  );
}
