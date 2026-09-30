import { FormEvent, useEffect, useState } from "react";
import {
  Agent,
  AgentSkill,
  listAgentSkills,
  setAgentSkill,
  updateAgent,
} from "../api";
import {
  SettingsCard,
  SettingsEmpty,
  SettingsHint,
  SettingsPage,
  SettingsSection,
} from "./SettingsLayout";

type Props = {
  agent: Agent | null;
  busy?: boolean;
  onSaved?: (agent: Agent) => void;
};

export function BotSettingsPanel({ agent, busy, onSaved }: Props) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [computerMode, setComputerMode] = useState<"team" | "private">("team");
  const [skills, setSkills] = useState<AgentSkill[]>([]);
  const [msg, setMsg] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!agent) {
      setName("");
      setDescription("");
      setSystemPrompt("");
      setComputerMode("team");
      setSkills([]);
      return;
    }
    setName(agent.name || "");
    setDescription(agent.description || "");
    setSystemPrompt(agent.system_prompt || "");
    setComputerMode(agent.computer_mode === "private" ? "private" : "team");
    let cancelled = false;
    void listAgentSkills(agent.id)
      .then((list) => {
        if (!cancelled) setSkills(list);
      })
      .catch((err) => {
        if (!cancelled) setMsg(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [agent?.id, agent?.name, agent?.description, agent?.system_prompt, agent?.computer_mode]);

  if (!agent) {
    return (
      <SettingsPage>
        <SettingsHint>从左侧选择一位助手后，可在此编辑岗位描述、电脑模式与本 Bot 启用的 Skills。</SettingsHint>
        <SettingsCard>
          <SettingsEmpty>未选择 Bot</SettingsEmpty>
        </SettingsCard>
      </SettingsPage>
    );
  }

  const onSave = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setMsg("");
    try {
      const updated = await updateAgent(agent.id, {
        name: name.trim() || agent.name,
        description: description.trim(),
        system_prompt: systemPrompt.trim(),
        computer_mode: computerMode,
      });
      onSaved?.(updated);
      setMsg("已保存岗位与电脑模式");
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const toggleSkill = async (sk: AgentSkill, enabled: boolean) => {
    setSaving(true);
    setMsg("");
    try {
      const next = await setAgentSkill(agent.id, sk.name, enabled);
      setSkills((prev) => prev.map((s) => (s.name === next.name ? next : s)));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const locked = Boolean(busy || saving);

  return (
    <SettingsPage>
      <SettingsHint>
        岗位描述会写入 Bot 资料；Skills 为本 Bot 允许列表（与账号级启用取交集）。电脑模式：team
        共享账户工作区，private 为该 Bot 独立目录。
      </SettingsHint>
      <SettingsSection title={`Bot · ${agent.name}`}>
        <SettingsCard padded>
          <form className="llm-form" onSubmit={(e) => void onSave(e)}>
            <label>
              名称
              <input value={name} onChange={(e) => setName(e.target.value)} disabled={locked} required />
            </label>
            <label>
              岗位描述
              <textarea
                rows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                disabled={locked}
                placeholder="这位助手负责什么"
              />
            </label>
            <label>
              系统提示（人设）
              <textarea
                rows={5}
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
                disabled={locked}
                placeholder="更细的行为约束"
              />
            </label>
            <label>
              电脑模式
              <select
                value={computerMode}
                onChange={(e) => setComputerMode(e.target.value as "team" | "private")}
                disabled={locked}
              >
                <option value="team">team · 账户共用文件</option>
                <option value="private">private · 本 Bot 私有文件</option>
              </select>
            </label>
            <div className="llm-actions">
              <button type="submit" className="primary" disabled={locked}>
                {saving ? "保存中…" : "保存"}
              </button>
            </div>
          </form>
        </SettingsCard>
      </SettingsSection>
      <SettingsSection title="本 Bot 启用的 Skills">
        <SettingsCard>
          <div className="llm-list">
            {skills.length === 0 ? (
              <SettingsEmpty>暂无技能（或尚未加载）。请先在「Skills」页安装/启用账号级技能。</SettingsEmpty>
            ) : (
              skills.map((s) => (
                <div key={s.name} className="llm-item">
                  <div>
                    <div className="agent-name">
                      {s.name}
                      {s.custom ? <span className="pill">自定义</span> : null}
                    </div>
                    <div className="agent-desc">{s.description}</div>
                  </div>
                  <div className="llm-actions">
                    <label className="check skill-toggle">
                      <input
                        type="checkbox"
                        checked={s.enabled}
                        disabled={locked}
                        onChange={(e) => void toggleSkill(s, e.target.checked)}
                      />
                      {s.enabled ? "本 Bot 启用" : "本 Bot 关闭"}
                    </label>
                  </div>
                </div>
              ))
            )}
          </div>
        </SettingsCard>
        {msg ? <div className="settings-status">{msg}</div> : null}
      </SettingsSection>
    </SettingsPage>
  );
}
