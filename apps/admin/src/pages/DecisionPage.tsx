import { useEffect, useState } from "react";
import { Button, Card, Form, Space, message, Typography } from "antd";
import {
  PageContainer,
  ProForm,
  ProFormSelect,
  ProFormText,
} from "@ant-design/pro-components";
import { adminGetDecision, adminPutDecision, adminTestDecision } from "../api";

const { Paragraph } = Typography;

const PROVIDER_HELP: Record<string, string> = {
  off: "关闭时聊天、工具和 A2A 都不会调用决策模型。",
  jev: "Jev 走托管接口 POST {base_url}/v1/systemone，默认 https://api.typesafe.ai ，模型 jev-latest，需要 API key。",
  laya: "填写 base_url 时，Laya 走同一套 HTTP 契约。留空则由 runtime 进程内懒加载 laya（需已 pip install laya），默认模型 convaiinnovations/laya。",
};

export default function DecisionPage() {
  const [hint, setHint] = useState("");
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);
  const [form] = Form.useForm();
  const provider = Form.useWatch("provider", form) || "off";

  useEffect(() => {
    setLoading(true);
    void adminGetDecision()
      .then((data) => {
        setHint(data.api_key_hint || "");
        form.setFieldsValue({
          provider: data.provider || "off",
          base_url: data.base_url || "",
          model: data.model || "",
          api_key: "",
        });
      })
      .catch((err) => {
        message.error(err instanceof Error ? err.message : String(err));
      })
      .finally(() => setLoading(false));
  }, [form]);

  async function reload() {
    const data = await adminGetDecision();
    setHint(data.api_key_hint || "");
    form.setFieldsValue({
      provider: data.provider || "off",
      base_url: data.base_url || "",
      model: data.model || "",
      api_key: "",
    });
  }

  return (
    <PageContainer title="决策模型">
      <Card loading={loading}>
        <Paragraph type="secondary">
          组织级 System One 适配层，默认关闭。业务只调用统一的 decide 接口，不直接依赖 Jev 或 Laya。
          {hint ? ` 当前密钥 ${hint}。` : ""}
        </Paragraph>
        <Paragraph type="secondary">{PROVIDER_HELP[provider] || PROVIDER_HELP.off}</Paragraph>
        <ProForm
          form={form}
          layout="vertical"
          style={{ maxWidth: 560 }}
          submitter={{
            searchConfig: { submitText: "保存" },
            render: (_, dom) => (
              <Space>
                <Button
                  loading={testing}
                  onClick={() => {
                    const values = form.getFieldsValue();
                    if ((values.provider || "off") === "off") {
                      message.info("当前为关闭，不会调用模型");
                      return;
                    }
                    setTesting(true);
                    void adminTestDecision({
                      provider: values.provider || "off",
                      base_url: values.base_url || "",
                      api_key: values.api_key || "",
                      model: values.model || "",
                    })
                      .then((data) => {
                        const noul = data.answers?.ok?.noul;
                        message.success(
                          typeof noul === "number"
                            ? `连接成功（${data.provider}，探测概率 ${noul.toFixed(2)}）`
                            : `连接成功（${data.provider || values.provider}）`,
                        );
                      })
                      .catch((err) => {
                        message.error(err instanceof Error ? err.message : String(err));
                      })
                      .finally(() => setTesting(false));
                  }}
                >
                  测试连接
                </Button>
                {dom}
              </Space>
            ),
          }}
          onFinish={async (values) => {
            try {
              await adminPutDecision({
                provider: values.provider || "off",
                base_url: values.base_url || "",
                api_key: values.api_key || "",
                model: values.model || "",
              });
              message.success("决策模型已保存");
              await reload();
              return true;
            } catch (err) {
              message.error(err instanceof Error ? err.message : String(err));
              return false;
            }
          }}
        >
          <ProFormSelect
            name="provider"
            label="提供方"
            initialValue="off"
            options={[
              { label: "关闭", value: "off" },
              { label: "Jev", value: "jev" },
              { label: "Laya", value: "laya" },
            ]}
            rules={[{ required: true, message: "请选择提供方" }]}
          />
          <ProFormText
            name="base_url"
            label="Base URL"
            placeholder={provider === "laya" ? "留空则使用本机 laya" : "https://api.typesafe.ai"}
          />
          <ProFormText.Password
            name="api_key"
            label="API Key（留空则保持原值）"
            fieldProps={{ autoComplete: "off" }}
          />
          <ProFormText
            name="model"
            label="模型"
            placeholder={provider === "laya" ? "convaiinnovations/laya" : "jev-latest"}
          />
        </ProForm>
      </Card>
    </PageContainer>
  );
}
