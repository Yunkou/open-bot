import { ExternalLink } from "lucide-react";
import { ADMIN_URL } from "../api";
import { SettingsCard, SettingsHint, SettingsPage, SettingsSection } from "./SettingsLayout";

/** Thin entry: full skills management lives in apps/admin (design v1.1). */
export function SkillsSettings() {
  return (
    <SettingsPage>
      <SettingsHint>
        技能包的创建、文件树编辑、导入导出已迁至<strong>管理端</strong>。主站设置不再保留完整管理 UI（
        <code>83d2abe</code> 落错面）。
      </SettingsHint>
      <SettingsSection title="去管理端编辑技能">
        <SettingsCard padded>
          <p style={{ margin: "0 0 12px", color: "#6b7280", fontSize: 13 }}>
            列表、VS Code 风文件树、多标签与保存校验均在管理端「技能」页。本 Bot 的启用勾选仍在「当前 Bot」。
          </p>
          <a
            className="primary"
            href={`${ADMIN_URL}/skills`}
            target="_blank"
            rel="noopener noreferrer"
            style={{ display: "inline-flex", alignItems: "center", gap: 6 }}
          >
            打开管理端 · 技能 <ExternalLink size={14} />
          </a>
        </SettingsCard>
      </SettingsSection>
    </SettingsPage>
  );
}
