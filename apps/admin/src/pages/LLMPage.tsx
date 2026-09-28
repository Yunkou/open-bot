import { useEffect, useState } from "react";
import { Card, Form, message, Typography } from "antd";
import {
  PageContainer,
  ProForm,
  ProFormDigit,
  ProFormSwitch,
  ProFormText,
} from "@ant-design/pro-components";
import { adminGetOrgLLM, adminPutOrgLLM, type OrgLLMSettings } from "../api";

const { Paragraph } = Typography;

export default function LLMPage() {
  const [llm, setLlm] = useState<OrgLLMSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [form] = Form.useForm();

  useEffect(() => {
    setLoading(true);
    void adminGetOrgLLM()
      .then((data) => {
        setLlm(data);
        form.setFieldsValue({
          name: data.llm_name || "",
          base_url: data.llm_base_url || "",
          api_key: "",
          model: data.llm_model || "",
          enable_tools: Boolean(data.llm_enable_tools),
          context_window: data.llm_context_window ?? undefined,
        });
      })
      .catch((err) => {
        message.error(err instanceof Error ? err.message : String(err));
      })
      .finally(() => setLoading(false));
  }, [form]);

  return (
    <PageContainer title="默认模型">
      <Card loading={loading}>
        <Paragraph type="secondary">
          成员无个人默认模型时，聊天使用此处配置（密钥脱敏回显
          {llm?.api_key_hint ? `：${llm.api_key_hint}` : ""}）。
          优先级：个人默认/首个连接 &gt; 组织默认 &gt; 无。
        </Paragraph>
        <ProForm
          form={form}
          layout="vertical"
          style={{ maxWidth: 560 }}
          submitter={{ searchConfig: { submitText: "保存" } }}
          onFinish={async (values) => {
            try {
              const cw = values.context_window;
              await adminPutOrgLLM({
                name: values.name || "",
                base_url: values.base_url || "",
                api_key: values.api_key || "",
                model: values.model || "",
                enable_tools: Boolean(values.enable_tools),
                context_window: cw && Number(cw) > 0 ? Number(cw) : null,
              });
              message.success("组织默认模型已保存");
              const data = await adminGetOrgLLM();
              setLlm(data);
              form.setFieldsValue({
                name: data.llm_name || "",
                base_url: data.llm_base_url || "",
                api_key: "",
                model: data.llm_model || "",
                enable_tools: Boolean(data.llm_enable_tools),
                context_window: data.llm_context_window ?? undefined,
              });
              return true;
            } catch (err) {
              message.error(err instanceof Error ? err.message : String(err));
              return false;
            }
          }}
        >
          <ProFormText name="name" label="名称" />
          <ProFormText name="base_url" label="Base URL" />
          <ProFormText.Password
            name="api_key"
            label="API Key（留空则保持原值）"
            fieldProps={{ autoComplete: "off" }}
          />
          <ProFormText name="model" label="模型" />
          <ProFormDigit
            name="context_window"
            label="上下文窗口 (tokens)"
            placeholder="可选"
            min={1}
            fieldProps={{ precision: 0 }}
          />
          <ProFormSwitch name="enable_tools" label="启用 tools" />
        </ProForm>
      </Card>
    </PageContainer>
  );
}
