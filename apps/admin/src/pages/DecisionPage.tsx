import { useEffect, useState } from "react";
import { Button, Card, Form, Space, message, Typography } from "antd";
import {
  PageContainer,
  ProForm,
  ProFormSelect,
  ProFormText,
} from "@ant-design/pro-components";
import { adminGetDecision, adminPutDecision, adminTestDecision } from "../api";

const { Paragraph, Text } = Typography;

const PROVIDER_HELP: Record<string, string> = {
  off: "关闭时聊天、工具和 A2A 都不会调用决策模型。",
  jev: "Jev 走托管接口 POST {base_url}/v1/systemone，默认 https://api.typesafe.ai ，模型 jev-latest，需要 API key。",
  laya: "填写 base_url 时，Laya 走同一套 HTTP 契约。留空则由 runtime 进程内懒加载 laya（需已 pip install laya），默认模型 convaiinnovations/laya。",
  unsloth:
    "Unsloth 桌面端本地 Decision API（TypeSafe/Jev 兼容）。默认 http://localhost:8888 ，模型可用 laya / jev-latest / laya-multilingual / laya-english / laya-typed-decisions。请先在 Unsloth：设置 → API → 决策 API 启用「提供请求服务」；本机可配置 API key，或开启无密钥访问后留空。置信度公式与云端 Jev 不同，业务侧请优先用 probabilities 阈值。",
};

export default function DecisionPage() {
  const [hint, setHint] = useState("");
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState(false);
  const [probeMsg, setProbeMsg] = useState("");
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

  const runModelProbe = () => {
    const values = form.getFieldsValue();
    if ((values.provider || "off") === "off") {
      setProbeMsg("当前为关闭，不会调用模型");
      message.info("当前为关闭，不会调用模型");
      return;
    }
    setTesting(true);
    setProbeMsg("正在检测模型可用性…");
    const started = performance.now();
    void adminTestDecision({
      provider: values.provider || "off",
      base_url: values.base_url || "",
      api_key: values.api_key || "",
      model: values.model || "",
    })
      .then((data) => {
        const latency = Math.round(performance.now() - started);
        const noul = data.answers?.ok?.noul;
        const modelLabel = data.model || values.model || "";
        const text =
          typeof noul === "number"
            ? `模型可用（${data.provider}${modelLabel ? ` · ${modelLabel}` : ""} · ${latency}ms，探测概率 ${noul.toFixed(2)}）`
            : `模型可用（${data.provider || values.provider}${modelLabel ? ` · ${modelLabel}` : ""} · ${latency}ms）`;
        setProbeMsg(text);
        message.success(text);
      })
      .catch((err) => {
        const latency = Math.round(performance.now() - started);
        const errText = err instanceof Error ? err.message : String(err);
        const text = `模型不可用（${latency}ms）：${errText}`;
        setProbeMsg(text);
        message.error(text);
      })
      .finally(() => setTesting(false));
  };

  return (
    <PageContainer title="决策模型">
      <Card loading={loading}>
        <Paragraph type="secondary">
          组织级 System One 适配层，默认关闭。业务只调用统一的 decide 接口，不直接依赖 Jev、Laya 或 Unsloth。
          {hint ? ` 当前密钥 ${hint}。` : ""}
          填写提供方、端点与模型后，可用「检测模型」确认该配置可响应（与默认模型页的 list-models / probe 一类探测同款反馈）。
        </Paragraph>
        <Paragraph type="secondary">{PROVIDER_HELP[provider] || PROVIDER_HELP.off}</Paragraph>
        <ProForm
          form={form}
          layout="vertical"
          style={{ maxWidth: 560 }}
          submitter={{
            searchConfig: { submitText: "保存" },
            render: (_, dom) => (
              <Space wrap>
                <Button loading={testing} onClick={() => void runModelProbe()}>
                  检测模型
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
              { label: "Unsloth（本地 Laya）", value: "unsloth" },
            ]}
            rules={[{ required: true, message: "请选择提供方" }]}
          />
          <ProFormText
            name="base_url"
            label="Base URL"
            placeholder={
              provider === "laya"
                ? "留空则使用本机 laya"
                : provider === "unsloth"
                  ? "http://localhost:8888"
                  : "https://api.typesafe.ai"
            }
          />
          <ProFormText.Password
            name="api_key"
            label="API Key（留空则保持原值）"
            fieldProps={{ autoComplete: "off" }}
          />
          <ProFormText
            name="model"
            label="模型"
            placeholder={
              provider === "laya"
                ? "convaiinnovations/laya"
                : provider === "unsloth"
                  ? "laya（或 jev-latest / laya-multilingual）"
                  : "jev-latest"
            }
          />
        </ProForm>
        {probeMsg ? (
          <Text type="secondary" style={{ display: "block", marginTop: 12 }}>
            {probeMsg}
          </Text>
        ) : null}
      </Card>
    </PageContainer>
  );
}
