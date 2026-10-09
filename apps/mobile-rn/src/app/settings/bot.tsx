import { Button, Card, Chip, Typography } from "heroui-native";
import type { JSX } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { ScrollView, View } from "react-native";

import * as api from "@/api";
import type { Agent, AgentSkill } from "@/api/types";
import { FormField, SectionTitle, SwitchRow } from "@/components/FormField";
import { ScreenScaffold } from "@/components/ScreenScaffold";
import { EmptyState } from "@/components/states";

/**
 * Bot 设置。对齐 Web 端 `settingsTab === "bot"`（`components/BotSettingsPanel.tsx`）：
 * 岗位资料 + 电脑模式 + 本 Bot 启用的技能。
 *
 * 与 Web 的结构差异只有一处，而且是必要的：Web 的「当前 Bot」是全局 sidebar 概念
 * （选中态住在 App 顶层，聊天页和设置页共用）。RN 这边没有跨页共享状态的既有设施，
 * 为一个下拉框引入状态管理库不划算，所以这里做成**页内切换**：
 * 顶部一排 Chip 选 Bot，选中态只在本页有效。
 */

const COMPUTER_MODES = [
  {
    value: "team" as const,
    label: "team",
    desc: "账户共用文件 · 同账户下多个 Bot 共享一份工作区",
  },
  {
    value: "private" as const,
    label: "private",
    desc: "本 Bot 私有文件 · 每个 Bot 各自一份目录",
  },
];

function errText(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

export default function BotSettingsScreen(): JSX.Element {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // 选中态同时存一份 ref：`load` 需要它来保持刷新后的选中项不变，
  // 但又不能把它放进依赖数组（那会让 load 每次渲染都变，进而让初始化 effect 反复跑）。
  const selectedRef = useRef<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [computerMode, setComputerMode] = useState<"team" | "private">("team");

  const [skills, setSkills] = useState<AgentSkill[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(false);

  /** 把服务端返回的 Agent 灌进表单。纯 setState，不含副作用。 */
  const fillForm = useCallback((agent: Agent) => {
    setName(agent.name || "");
    setDescription(agent.description || "");
    setSystemPrompt(agent.system_prompt || "");
    setComputerMode(agent.computer_mode === "private" ? "private" : "team");
  }, []);

  const loadSkills = useCallback(async (agentId: string) => {
    setSkillsLoading(true);
    try {
      setSkills(await api.listAgentSkills(agentId));
    } catch (err) {
      setSkills([]);
      setMsg(errText(err, "加载技能失败"));
    } finally {
      setSkillsLoading(false);
    }
  }, []);

  const selectAgent = useCallback(
    (agent: Agent) => {
      selectedRef.current = agent.id;
      setSelectedId(agent.id);
      fillForm(agent);
      setMsg("");
      void loadSkills(agent.id);
    },
    [fillForm, loadSkills]
  );

  const load = useCallback(async () => {
    try {
      setError(null);
      const list = await api.listAgents();
      setAgents(list);
      // 刷新后尽量保持当前选中项；没有（或已被删）就退回第一个
      const target = list.find((a) => a.id === selectedRef.current) ?? list[0];
      if (target) {
        selectedRef.current = target.id;
        setSelectedId(target.id);
        fillForm(target);
        await loadSkills(target.id);
      }
    } catch (err) {
      setError(errText(err, "加载助手失败"));
    } finally {
      setLoading(false);
    }
  }, [fillForm, loadSkills]);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  async function save(): Promise<void> {
    if (!selectedId) return;
    const trimmed = name.trim();
    if (!trimmed) {
      setMsg("名称必填");
      return;
    }
    setBusy(true);
    setMsg("");
    try {
      await api.updateAgent(selectedId, {
        name: trimmed,
        description: description.trim(),
        system_prompt: systemPrompt.trim(),
        computer_mode: computerMode,
      });
      setMsg("已保存岗位与电脑模式");
      await load();
    } catch (err) {
      setMsg(errText(err, "保存失败"));
    } finally {
      setBusy(false);
    }
  }

  async function toggleSkill(sk: AgentSkill, enabled: boolean): Promise<void> {
    if (!selectedId) return;
    setBusy(true);
    setMsg("");
    try {
      const next = await api.setAgentSkill(selectedId, sk.name, enabled);
      setSkills((prev) => prev.map((s) => (s.name === next.name ? next : s)));
    } catch (err) {
      setMsg(errText(err, "更新技能失败"));
    } finally {
      setBusy(false);
    }
  }

  const agent = agents.find((a) => a.id === selectedId) ?? null;
  // 账号级已关闭的技能不给开关：那是「技能」页管的维度，
  // 在这里摆一个拨不动的开关只会让人以为是自己关的（与 Web 同口径）。
  const visibleSkills = skills.filter((s) => s.account_enabled !== false);
  const hiddenCount = skills.length - visibleSkills.length;

  return (
    <ScreenScaffold
      title="Bot 设置"
      subtitle="岗位描述、电脑模式与启用技能"
      loading={loading}
      error={error}
      emptyOnly={agents.length === 0}
      empty={
        <EmptyState
          icon="sparkles-outline"
          title="还没有助手"
          hint="先在聊天页新建一个助手，才能在这里配置它的岗位与技能"
        />
      }
      onRetry={() => void load()}
    >
      {agents.length > 1 ? (
        <View className="gap-2.5">
          <SectionTitle>选择 Bot</SectionTitle>
          <ScrollView horizontal showsHorizontalScrollIndicator={false}>
            <View className="flex-row gap-2 pr-2">
              {agents.map((a) => (
                <Chip
                  key={a.id}
                  size="sm"
                  variant={a.id === selectedId ? "soft" : "secondary"}
                  color={a.id === selectedId ? "accent" : "default"}
                  onPress={() => selectAgent(a)}
                  accessibilityRole="button"
                  accessibilityLabel={`编辑 ${a.name}`}
                >
                  <Chip.Label>{a.name}</Chip.Label>
                </Chip>
              ))}
            </View>
          </ScrollView>
        </View>
      ) : null}

      {!agent ? (
        <EmptyState
          icon="sparkles-outline"
          title="未选择 Bot"
          hint="选择一位助手后，可在此编辑岗位描述、电脑模式与本 Bot 启用的技能"
        />
      ) : (
        <>
          <Card>
            <Card.Body className="gap-4">
              <SectionTitle>Bot · {agent.name}</SectionTitle>

              <FormField
                label="名称"
                required
                value={name}
                onChangeText={setName}
                placeholder="写作助手"
                editable={!busy}
              />
              <FormField
                label="岗位描述"
                value={description}
                onChangeText={setDescription}
                placeholder="这位助手负责什么"
                editable={!busy}
                multiline
              />
              <FormField
                label="系统提示（人设）"
                value={systemPrompt}
                onChangeText={setSystemPrompt}
                placeholder="更细的行为约束"
                editable={!busy}
                multiline
                hint="留空则使用默认人设；技能正文请到「Skills」页编辑"
              />

              <View className="gap-2">
                <SectionTitle>电脑模式</SectionTitle>
                <ScrollView horizontal showsHorizontalScrollIndicator={false}>
                  <View className="flex-row gap-2 pr-2">
                    {COMPUTER_MODES.map((opt) => (
                      <Chip
                        key={opt.value}
                        size="sm"
                        variant={computerMode === opt.value ? "soft" : "secondary"}
                        color={computerMode === opt.value ? "accent" : "default"}
                        disabled={busy}
                        onPress={() => setComputerMode(opt.value)}
                        accessibilityRole="button"
                        accessibilityLabel={opt.desc}
                      >
                        <Chip.Label>{opt.label}</Chip.Label>
                      </Chip>
                    ))}
                  </View>
                </ScrollView>
                <Typography.Paragraph color="muted" className="text-xs">
                  {COMPUTER_MODES.find((o) => o.value === computerMode)?.desc}
                </Typography.Paragraph>
              </View>

              <Button isDisabled={busy} onPress={() => void save()}>
                <Button.Label>{busy ? "保存中…" : "保存"}</Button.Label>
              </Button>
            </Card.Body>
          </Card>

          <View className="gap-3">
            <SectionTitle>本 Bot 启用的技能</SectionTitle>
            {skillsLoading ? (
              <Typography.Paragraph color="muted">正在加载技能…</Typography.Paragraph>
            ) : visibleSkills.length === 0 ? (
              <Typography.Paragraph color="muted">
                暂无可用技能（或尚未加载）。请先在管理端启用平台技能，并在账号侧保持可用。
              </Typography.Paragraph>
            ) : (
              <Card>
                <Card.Body className="gap-1">
                  {visibleSkills.map((s) => (
                    <SwitchRow
                      key={s.name}
                      label={s.name}
                      description={s.description}
                      value={s.enabled}
                      disabled={busy}
                      onValueChange={(next) => void toggleSkill(s, next)}
                      right={
                        s.custom ? (
                          <Chip size="sm" variant="soft" color="accent">
                            <Chip.Label>自建</Chip.Label>
                          </Chip>
                        ) : null
                      }
                    />
                  ))}
                </Card.Body>
              </Card>
            )}

            {hiddenCount > 0 ? (
              <Typography.Paragraph color="muted" className="text-xs">
                另有 {hiddenCount} 个技能已在「Skills」页账号级关闭，不可勾选。
              </Typography.Paragraph>
            ) : null}
          </View>

          {msg ? <Typography.Paragraph color="muted">{msg}</Typography.Paragraph> : null}
        </>
      )}
    </ScreenScaffold>
  );
}
