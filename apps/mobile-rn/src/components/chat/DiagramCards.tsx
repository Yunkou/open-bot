/**
 * 图表卡：Mermaid / HTML / 图片。
 *
 * 对齐 Web 端的 `DiagramCard` + `DiagramCardFrame` + `HtmlDiagramCard` + `ImageDiagramCard`，
 * 但有一处**刻意的能力裁剪**：
 *
 * **RN 端不渲染 Mermaid。** Web 端在浏览器里跑 mermaid 运行时（`lib/mermaidRender.ts`），
 * RN 里没有等价物 —— 塞一个 mermaid + jsdom 进 bundle 既重又脆（还要自己补
 * getBBox / DOMParser 等一堆 polyfill）。所以这里的 Mermaid 卡只展示源码，
 * 要看图得导出。这个取舍写在这里，免得后来的人以为是遗漏。
 *
 * 工具条折叠沿用 `DiagramCardFrame` 的思路（放不下就收进 ⋯），实现换成 RN 的
 * `onLayout` 量宽 + heroui `Menu`：RN 没有 ResizeObserver，也不该在布局阶段同步测量。
 */

import { Chip, Menu, Typography } from "heroui-native";
import * as Clipboard from "expo-clipboard";
import * as WebBrowser from "expo-web-browser";
import { useCallback, useEffect, useMemo, useState, type JSX, type ReactNode } from "react";
import { Image, Modal, Pressable, ScrollView, useColorScheme, View } from "react-native";

import { attachmentCopyUrl, attachmentDisplayUrl, imageDownloadFilename } from "@/api";
import type { AttachmentMeta } from "@/api/types";
import { Icon } from "@/components/Icon";
import { WebPreviewModal } from "@/components/chat/WebPreviewModal";
import { saveAttachment, saveTextFile } from "@/lib/download";

type IconName = Parameters<typeof Icon>[0]["name"];

/** 工具条动作。收起状态下进 ⋯ 菜单，展示的是同一份定义。 */
export type DiagramAction = {
  key: string;
  label: string;
  icon?: IconName;
  disabled?: boolean;
  onPress: () => void;
};

/* ------------------------------------------------------------------ *
 * 围栏识别
 * ------------------------------------------------------------------ */

/**
 * 从消息正文里取出一个围栏代码块。
 *
 * `pending` = 围栏还没闭合（流式输出中）。此时 HTML 预览与 Mermaid 导出都不该跑：
 * 半截源码渲出来的东西是错的，调用方拿到 pending 应该先按普通代码块显示。
 */
export function extractFence(
  content: string,
  lang: "mermaid" | "html"
): { source: string; pending: boolean } | null {
  const fence = new RegExp("^ {0,3}(`{3,}|~{3,})\\s*" + lang + "\\s*$", "im");
  const m = fence.exec(content || "");
  if (!m || m.index === undefined) return null;
  const marker = m[1]!;
  const rest = content.slice(m.index + m[0].length + 1);
  const close = new RegExp("^ {0,3}" + marker[0] + "{" + marker.length + ",}\\s*$", "m");
  const end = close.exec(rest);
  const body = end ? rest.slice(0, end.index) : rest;
  const source = body.replace(/\n$/, "");
  return { source, pending: !end };
}

/* ------------------------------------------------------------------ *
 * Mermaid 导出
 * ------------------------------------------------------------------ */

const BASE64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

/**
 * UTF-8 → base64url。
 *
 * 手写而不是用 `Buffer` / `btoa` / `TextEncoder`：这三个在 RN 里要么没装，要么取决于
 * Hermes 版本。中文标签必须按 UTF-8 多字节算，直接对 charCode 取模会得到错误结果。
 */
function utf8ToBase64Url(input: string): string {
  const bytes: number[] = [];
  for (let i = 0; i < input.length; i += 1) {
    let code = input.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff && i + 1 < input.length) {
      const next = input.charCodeAt(i + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        code = (code - 0xd800) * 0x400 + (next - 0xdc00) + 0x10000;
        i += 1;
      }
    }
    if (code < 0x80) bytes.push(code);
    else if (code < 0x800) bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
    else if (code < 0x10000) {
      bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
    } else {
      bytes.push(
        0xf0 | (code >> 18),
        0x80 | ((code >> 12) & 0x3f),
        0x80 | ((code >> 6) & 0x3f),
        0x80 | (code & 0x3f)
      );
    }
  }

  let out = "";
  for (let i = 0; i < bytes.length; i += 3) {
    const b0 = bytes[i]!;
    const b1 = bytes[i + 1];
    const b2 = bytes[i + 2];
    out += BASE64[b0 >> 2];
    out += BASE64[((b0 & 3) << 4) | ((b1 ?? 0) >> 4)];
    out += b1 === undefined ? "" : BASE64[((b1 & 15) << 2) | ((b2 ?? 0) >> 6)];
    out += b2 === undefined ? "" : BASE64[b2 & 63];
  }
  return out;
}

/**
 * mermaid.ink 渲染地址。
 *
 * ── 这是第三方服务，取舍写清楚 ────────────────────────────────────────
 * - **会把整段图表源码 POST/GET 给 mermaid.ink**（kroki 系的公开实例）。图里的文字
 *   等于离开了本机。这是移动端唯一不需要在 bundle 里塞 mermaid 运行时的办法；
 *   真要本地渲染就得引入 mermaid + DOM polyfill，代价远大于收益。
 * - **离线 / 服务不可用时导出直接失败**，大图偶尔 503。所以「导出」不等于「一定有结果」，
 *   卡上同时保留「复制源码」这条永远可用的路。
 * - 编码方式是 `base64(JSON.stringify({ code, mermaid: { theme } }))`：
 *   JSON.stringify 会把非 ASCII 转成 `\uXXXX`，服务端才能还原中文标签；
 *   base64 用 **url-safe 变体且去掉 `=` padding** —— 标准 base64 里的 `/` 在路径段里
 *   会被路由吃掉（实测 404），`+` 虽然能过但没必要冒险。
 */
export function mermaidInkUrl(
  source: string,
  opts?: { theme?: "default" | "dark"; svg?: boolean }
): string {
  const theme = opts?.theme === "dark" ? "dark" : "default";
  const state = JSON.stringify({ code: source, mermaid: { theme } });
  const kind = opts?.svg === false ? "img" : "svg";
  return `https://mermaid.ink/${kind}/${utf8ToBase64Url(state)}`;
}

/* ------------------------------------------------------------------ *
 * 共用外壳
 * ------------------------------------------------------------------ */

/** 工具条放不下 inline 动作时的折叠阈值。低于它就只留一个 ⋯。 */
const COLLAPSE_WIDTH = 240;

function DiagramFrame({
  label,
  extra,
  actions,
  children,
}: {
  label: string;
  /** 工具条上的附加内容（例如「预览 / 源码」切换） */
  extra?: ReactNode;
  actions: DiagramAction[];
  children: ReactNode;
}): JSX.Element {
  const [width, setWidth] = useState(0);
  const collapsed = width > 0 && width < COLLAPSE_WIDTH && actions.length > 1;

  return (
    <View
      className="gap-2 overflow-hidden rounded-2xl border border-border bg-background p-2.5"
      onLayout={(e) => setWidth(e.nativeEvent.layout.width)}
    >
      <View className="flex-row items-center gap-1.5">
        <View className="rounded-md bg-surface-secondary px-1.5 py-0.5">
          <Typography.Paragraph className="text-[10px] text-muted">{label}</Typography.Paragraph>
        </View>
        {extra ? <View className="flex-row items-center">{extra}</View> : null}
        <View className="flex-1" />
        {collapsed ? (
          <Menu presentation="popover">
            <Menu.Trigger>
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="更多操作"
                hitSlop={6}
                className="size-7 items-center justify-center rounded-md"
              >
                <Icon name="ellipsis-horizontal" size={16} tone="muted" />
              </Pressable>
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Overlay />
              <Menu.Content presentation="popover" placement="bottom" align="end" width={220}>
                {actions.map((a) => (
                  <Menu.Item key={a.key} isDisabled={a.disabled} onPress={a.onPress}>
                    <Menu.ItemTitle>{a.label}</Menu.ItemTitle>
                  </Menu.Item>
                ))}
              </Menu.Content>
            </Menu.Portal>
          </Menu>
        ) : (
          <View className="flex-row items-center gap-0.5">
            {actions.map((a) => (
              <Pressable
                key={a.key}
                accessibilityRole="button"
                accessibilityLabel={a.label}
                accessibilityState={{ disabled: Boolean(a.disabled) }}
                disabled={a.disabled}
                hitSlop={6}
                onPress={a.onPress}
                className="size-7 items-center justify-center rounded-md"
              >
                <Icon name={a.icon ?? "ellipsis-horizontal"} size={16} tone="muted" />
              </Pressable>
            ))}
          </View>
        )}
      </View>

      {children}
    </View>
  );
}

/** 等宽源码块：纵向封顶 + 横向滚动，和 `Markdown.tsx` 里的代码块保持同一套观感。 */
function SourceBlock({ source, maxHeight }: { source: string; maxHeight?: string }): JSX.Element {
  return (
    <View className={`rounded-xl bg-surface-secondary px-2.5 py-2 ${maxHeight ?? "max-h-52"}`}>
      <ScrollView nestedScrollEnabled>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <Typography.Paragraph
            selectable
            className="text-[11px]"
            style={{ fontFamily: "Menlo", lineHeight: 16 }}
          >
            {source}
          </Typography.Paragraph>
        </ScrollView>
      </ScrollView>
    </View>
  );
}

/* ------------------------------------------------------------------ *
 * Mermaid
 * ------------------------------------------------------------------ */

export function MermaidDiagramCard({
  source,
  pending,
}: {
  /** ```mermaid 围栏里的源码 */
  source: string;
  /** 流式输出中围栏未闭合 → 禁用导出，避免把半截图发给第三方 */
  pending?: boolean;
}): JSX.Element {
  const scheme = useColorScheme();
  // 记下预览是给哪份源码开的：源码一变（流式追加）就对不上，自动关闭，
  // 不需要「监听 source 再 setState(false)」那种重渲染。
  const [previewSource, setPreviewSource] = useState<string | null>(null);
  const preview = previewSource !== null && previewSource === source && !pending;

  const url = useMemo(
    () => (pending ? "" : mermaidInkUrl(source, { theme: scheme === "dark" ? "dark" : "default" })),
    [pending, scheme, source]
  );

  const copySource = useCallback(() => {
    void Clipboard.setStringAsync(source);
  }, [source]);

  const openExternal = useCallback(() => {
    if (url) void WebBrowser.openBrowserAsync(url);
  }, [url]);

  const actions: DiagramAction[] = [
    { key: "copy", label: "复制源码", icon: "copy-outline", onPress: copySource },
    {
      key: "svg",
      label: "导出 SVG",
      icon: "image-outline",
      disabled: pending || !url,
      onPress: () => setPreviewSource(source),
    },
    {
      key: "browser",
      label: "在浏览器中打开",
      icon: "open-outline",
      disabled: pending || !url,
      onPress: openExternal,
    },
  ];

  return (
    <>
      <DiagramFrame label="Mermaid" actions={actions}>
        <Typography.Paragraph className="text-[10px] text-muted">
          移动端不渲染 Mermaid，可复制源码或导出后在浏览器查看
        </Typography.Paragraph>
        <SourceBlock source={source || (pending ? "（生成中…）" : "（空）")} />
      </DiagramFrame>

      <WebPreviewModal
        visible={preview}
        mode="url"
        uri={url}
        title="Mermaid 图表"
        onClose={() => setPreviewSource(null)}
      />
    </>
  );
}

/* ------------------------------------------------------------------ *
 * HTML
 * ------------------------------------------------------------------ */

export function HtmlDiagramCard({
  source,
  pending,
}: {
  /** ```html 围栏里的源码 */
  source: string;
  pending?: boolean;
}): JSX.Element {
  const [showSource, setShowSource] = useState(true);
  // 同 Mermaid：预览记住是哪份源码开的，源码一变自动失效，不用 effect 去重置
  const [previewSource, setPreviewSource] = useState<string | null>(null);
  const preview = previewSource !== null && previewSource === source && !pending;

  const actions: DiagramAction[] = [
    {
      key: "copy",
      label: "复制源码",
      icon: "copy-outline",
      onPress: () => {
        void Clipboard.setStringAsync(source);
      },
    },
    {
      key: "save",
      label: "导出源码文件",
      icon: "download-outline",
      disabled: pending || !source,
      onPress: () => {
        // 预览要靠 WebView，源码要能带走：落成 .html 交给系统分享面板，
        // 用户可以直接丢进浏览器打开。
        void saveTextFile("diagram.html", source, "text/html;charset=utf-8");
      },
    },
  ];

  return (
    <>
      <DiagramFrame
        label="HTML"
        extra={
          <Chip
            size="sm"
            variant={showSource ? "secondary" : "soft"}
            color={showSource ? "default" : "accent"}
            onPress={() => setShowSource((v) => !v)}
            accessibilityLabel={showSource ? "切到预览" : "切到源码"}
          >
            <Chip.Label>{showSource ? "源码" : "预览"}</Chip.Label>
          </Chip>
        }
        actions={actions}
      >
        {showSource ? (
          <SourceBlock source={source || "（空）"} maxHeight="max-h-40" />
        ) : (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel="打开 HTML 预览"
            disabled={pending}
            onPress={() => setPreviewSource(source)}
            className="items-center justify-center gap-1 rounded-xl bg-surface-secondary py-5"
          >
            <Icon name="play-circle-outline" size={22} tone="accent" />
            <Typography.Paragraph className="text-[11px] text-accent">
              {pending ? "生成中…" : "点击打开预览"}
            </Typography.Paragraph>
          </Pressable>
        )}
      </DiagramFrame>

      <WebPreviewModal
        visible={preview}
        mode="html"
        html={source}
        title="HTML 预览"
        onClose={() => setPreviewSource(null)}
      />
    </>
  );
}

/* ------------------------------------------------------------------ *
 * 图片
 * ------------------------------------------------------------------ */

export function ImageDiagramCard({
  attachment,
  alt,
}: {
  /** 图片附件；地址由组件自己换取（含鉴权 token），调用方只管把 meta 传进来 */
  attachment: AttachmentMeta;
  alt?: string;
}): JSX.Element {
  const [fullscreen, setFullscreen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // 附件 GET 要鉴权，地址是异步换来的（token 拼在 query 上，RN 的 Image 没法注入请求头）。
  // 取址结果带上发起时的 id：换了附件或点了重试就自然对不上，回落到「加载中」，
  // 不用在 effect 里同步 setState 去手动重置。
  const [retryKey, setRetryKey] = useState(0);
  const requestId = `${attachment.id}:${retryKey}`;
  const [fetched, setFetched] = useState<{
    id: string;
    url: string | null;
    status: "loading" | "error";
  }>({ id: "", url: null, status: "loading" });

  const displayUrl = fetched.id === requestId ? fetched.url : null;
  const status = fetched.id === requestId ? fetched.status : "loading";

  useEffect(() => {
    let alive = true;
    void attachmentDisplayUrl(attachment)
      .then((url) => {
        if (!alive) return;
        setFetched({ id: requestId, url, status: url ? "loading" : "error" });
      })
      .catch(() => {
        if (!alive) return;
        setFetched({ id: requestId, url: null, status: "error" });
      });
    return () => {
      alive = false;
    };
  }, [attachment, requestId]);

  const [imgStatus, setImgStatus] = useState<"loading" | "ok" | "error">("loading");

  const retry = useCallback(() => {
    setError(null);
    setImgStatus("loading");
    setRetryKey((k) => k + 1);
  }, []);

  const copyUrl = attachmentCopyUrl(attachment);

  const copyLink = useCallback(() => {
    if (copyUrl) void Clipboard.setStringAsync(copyUrl);
  }, [copyUrl]);

  const download = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      // 交给系统分享面板，用户可存到相册/文件。文件名走统一的命名规则，不带 token。
      await saveAttachment({
        url: attachment.url,
        name: imageDownloadFilename(attachment.name, attachment.mime),
        mime: attachment.mime,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "下载失败，请重试");
    } finally {
      setBusy(false);
    }
  }, [attachment.mime, attachment.name, attachment.url]);

  const actions: DiagramAction[] = [
    {
      key: "fullscreen",
      label: "全屏查看",
      icon: "expand-outline",
      onPress: () => setFullscreen(true),
    },
    { key: "copy", label: "复制地址", icon: "link-outline", disabled: !copyUrl, onPress: copyLink },
    {
      key: "download",
      label: "下载原图",
      icon: "download-outline",
      disabled: busy,
      onPress: () => void download(),
    },
  ];

  return (
    <>
      <DiagramFrame label="图片" actions={actions}>
        {!displayUrl ? (
          <View className="items-center gap-2 rounded-xl bg-surface-secondary py-6">
            <Icon name="image-outline" size={22} tone="muted" />
            <Typography.Paragraph className="text-[11px] text-muted">
              {status === "error" ? "图片地址不可用" : "正在取图片地址…"}
            </Typography.Paragraph>
            {status === "error" ? (
              <Chip size="sm" variant="secondary" color="default" onPress={retry}>
                <Chip.Label>重试</Chip.Label>
              </Chip>
            ) : null}
          </View>
        ) : imgStatus === "error" ? (
          <View className="items-center gap-2 rounded-xl bg-surface-secondary py-6">
            <Icon name="image-outline" size={22} tone="muted" />
            <Typography.Paragraph className="text-[11px] text-muted">
              图片加载失败
            </Typography.Paragraph>
            <Chip size="sm" variant="secondary" color="default" onPress={retry}>
              <Chip.Label>重试</Chip.Label>
            </Chip>
          </View>
        ) : (
          <Pressable
            accessibilityRole="imagebutton"
            accessibilityLabel={alt || attachment.name || "图片"}
            onPress={() => setFullscreen(true)}
            className="overflow-hidden rounded-xl bg-surface-secondary"
          >
            <Image
              key={`${displayUrl}#${retryKey}`}
              source={{ uri: displayUrl }}
              style={{ width: "100%", height: 200 }}
              resizeMode="contain"
              onLoad={() => setImgStatus("ok")}
              onError={() => setImgStatus("error")}
              accessibilityLabel={alt || attachment.name || "图片"}
            />
          </Pressable>
        )}

        {error ? (
          <Typography.Paragraph className="text-[11px] text-danger">{error}</Typography.Paragraph>
        ) : null}
      </DiagramFrame>

      <Modal
        visible={fullscreen && Boolean(displayUrl)}
        animationType="fade"
        presentationStyle="fullScreen"
        onRequestClose={() => setFullscreen(false)}
      >
        <View className="flex-1 bg-black">
          <View className="flex-row items-center justify-end gap-1 px-2 pt-safe-offset-3 pb-2">
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="复制图片地址"
              disabled={!copyUrl}
              hitSlop={8}
              onPress={copyLink}
              className="size-9 items-center justify-center"
            >
              <Icon name="link-outline" size={20} tone="accent-foreground" />
            </Pressable>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="下载原图"
              disabled={busy}
              hitSlop={8}
              onPress={() => void download()}
              className="size-9 items-center justify-center"
            >
              <Icon name="download-outline" size={20} tone="accent-foreground" />
            </Pressable>
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="关闭"
              hitSlop={8}
              onPress={() => setFullscreen(false)}
              className="size-9 items-center justify-center"
            >
              <Icon name="close" size={22} tone="accent-foreground" />
            </Pressable>
          </View>
          <View className="flex-1 items-center justify-center">
            <Image
              source={{ uri: displayUrl ?? "" }}
              style={{ width: "100%", height: "100%" }}
              resizeMode="contain"
              accessibilityLabel={alt || attachment.name || "图片"}
            />
          </View>
          <View className="px-4 pb-safe-or-2">
            <Typography.Paragraph className="text-center text-[11px] text-muted" numberOfLines={1}>
              {attachment.name}
            </Typography.Paragraph>
          </View>
        </View>
      </Modal>
    </>
  );
}
