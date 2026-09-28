export type HostConfirmPayload = {
  req_id: string;
  op: string;
  path: string;
  dest?: string;
  preview?: string;
  status: "pending" | "allowed" | "denied" | string;
};

export function parseHostConfirm(content: string): HostConfirmPayload | null {
  try {
    const data = JSON.parse(content) as HostConfirmPayload;
    if (!data?.req_id || !data.op) return null;
    return data;
  } catch {
    return null;
  }
}

export function hostConfirmReqId(content: string): string {
  return parseHostConfirm(content)?.req_id || "";
}

function actionCopy(item: HostConfirmPayload): { title: string; detail: string; danger: boolean } {
  if (item.op === "shell") {
    return {
      title: item.dest === "terminal" ? "在终端里运行？" : "运行这条命令？",
      detail: item.path,
      danger: true,
    };
  }
  if (item.op === "delete") {
    return { title: "删除这个文件？", detail: item.path, danger: true };
  }
  if (item.op === "move") {
    return {
      title: "移动或重命名？",
      detail: item.dest ? `${item.path} → ${item.dest}` : item.path,
      danger: true,
    };
  }
  return { title: "覆盖已有文件？", detail: item.path, danger: false };
}

export function HostConfirmCard({
  item,
  onDecide,
}: {
  item: HostConfirmPayload;
  onDecide?: (ok: boolean) => void;
}) {
  const { title, detail, danger } = actionCopy(item);
  const pending = item.status === "pending";
  const settled = item.status === "allowed" ? "已允许" : item.status === "denied" ? "已拒绝" : "";

  return (
    <div className="host-confirm-card" role="region" aria-label={title}>
      <div className="host-confirm-kicker">需要你确认</div>
      <div className="host-confirm-title">{title}</div>
      {detail ? <div className="host-confirm-path">{detail}</div> : null}
      {item.preview && (item.op === "write" || item.op === "shell") ? (
        <pre className="host-confirm-preview">{item.preview}</pre>
      ) : null}
      {pending && onDecide ? (
        <div className="host-confirm-actions">
          <button type="button" className="ghost" onClick={() => onDecide(false)}>
            拒绝
          </button>
          <button
            type="button"
            className={danger ? "host-confirm-allow danger" : "primary"}
            onClick={() => onDecide(true)}
          >
            允许
          </button>
        </div>
      ) : pending ? (
        <div className="host-confirm-settled">等待确认</div>
      ) : (
        <div className={`host-confirm-settled${item.status === "denied" ? " denied" : ""}`}>{settled}</div>
      )}
    </div>
  );
}
