/**
 * Shared diagram-card shell (pill + toolbar + body). Used by Mermaid / HTML / 图片 cards.
 */
import type { ReactNode } from "react";

export type DiagramKind = "mermaid" | "html" | "image";

export const KIND_LABEL: Record<DiagramKind, string> = {
  mermaid: "Mermaid",
  html: "HTML",
  image: "图片",
};

export function DiagramCardFrame({
  kind,
  pending,
  extra,
  actions,
  children,
}: {
  kind: DiagramKind;
  pending?: boolean;
  extra?: ReactNode;
  actions: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className={`diagram-card${pending ? " is-pending" : ""}`} data-kind={kind}>
      <div className="diagram-toolbar">
        <span className="diagram-pill">{KIND_LABEL[kind]}</span>
        {extra}
        <div className="diagram-actions">{actions}</div>
      </div>
      {children}
    </div>
  );
}
