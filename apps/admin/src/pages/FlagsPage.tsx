import { useEffect, useState } from "react";
import { Card, Form, message, Typography } from "antd";
import { PageContainer, ProForm, ProFormTextArea } from "@ant-design/pro-components";
import { adminGetFeatureFlags, adminPutFeatureFlags } from "../api";

const { Paragraph } = Typography;

export default function FlagsPage() {
  const [loading, setLoading] = useState(true);
  const [form] = Form.useForm();

  useEffect(() => {
    setLoading(true);
    void adminGetFeatureFlags()
      .then((data) => {
        form.setFieldsValue({ feature_flags_json: data.feature_flags_json || "{}" });
      })
      .catch((err) => {
        message.error(err instanceof Error ? err.message : String(err));
      })
      .finally(() => setLoading(false));
  }, [form]);

  return (
    <PageContainer title="功能开关">
      <Card loading={loading}>
        <Paragraph type="secondary">JSON 存储，后续可接真实开关。</Paragraph>
        <ProForm
          form={form}
          layout="vertical"
          submitter={{ searchConfig: { submitText: "保存" } }}
          onFinish={async (values) => {
            try {
              const raw = String(values.feature_flags_json || "{}");
              JSON.parse(raw);
              const data = await adminPutFeatureFlags(raw);
              form.setFieldsValue({ feature_flags_json: data.feature_flags_json || "{}" });
              message.success("功能开关已保存");
              return true;
            } catch (err) {
              message.error(err instanceof Error ? err.message : "JSON 无效");
              return false;
            }
          }}
        >
          <ProFormTextArea
            name="feature_flags_json"
            fieldProps={{
              rows: 12,
              style: { fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace" },
            }}
            rules={[{ required: true, message: "请输入 JSON" }]}
          />
        </ProForm>
      </Card>
    </PageContainer>
  );
}
