import { FormEvent, MouseEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import { useConfirm } from "./components/ConfirmProvider";
import {
  Agent,
  API_BASE,
  Channel,
  CompactConfig,
  Conversation,
  LLMConnection,
  LLMInput,
  Message,
  Skill,
  User,
  clearSession,
  createAgent,
  createChannel,
  openPrimaryConversation,
  createLLMConnection,
  createMCPServer,
  deleteAgent,
  deleteChannel,
  deleteLLMConnection,
  deleteMCPServer,
  fetchCompactConfig,
  getStoredUser,
  getToken,
  listAgents,
  listChannels,
  listLLMConnections,
  listMCPServers,
  listMessages,
  listMessagesWithStatus,
  getConversationRunStatus,
  listSkills,
  uploadSkill,
  deleteSkill,
  login,
  mcpCallTool,
  register,
  fetchOIDCConfig,
  startOIDCLogin,
  sendMessageStream,
  subscribeConversationEvents,
  cancelConversationRun,
  chatEventsWebSocketUrl,
  mergeIncomingMessage,
  createHostConfirm,
  decideHostConfirm,
  uploadConversationAttachment,
  AttachmentMeta,
  setDefaultLLMConnection,
  setSession,
  setSkillEnabled,
  testMCPServer,
  updateLLMConnection,
  updateMCPServer,
  MCPServer,
  MCPServerInput,
  Routine,
  RoutineInput,
  listRoutines,
  createRoutine,
  updateRoutine,
  deleteRoutine,
  runRoutine,
  Sandbox,
  getSandbox,
  ensureSandbox,
  openChannelConversation,
  sandboxDesktopURL,
  stopSandbox,
  resetSandbox,
  execSandbox,
  listSandbox,
  readSandboxFile,
  writeSandboxFile,
  listBotSecrets,
  createBotSecret,
  deleteBotSecret,
  listBotSecretRequests,
  checkpointSandbox,
  listMachines,
  registerMachine,
  heartbeatMachine,
  deleteMachine,
  type Machine,
  type BotSecretMeta,
  type BotSecretRequest,
} from "./api";
import {
  detectClientContext,
  shouldRegisterAsHost,
  getOrCreateMachineKey,
  getStoredMachineId,
  setStoredMachineId,
  clearStoredMachineId,
  defaultMachineLabel,
} from "./lib/clientEnv";
import { startHostExecSession, hostWritesEnabled, setHostWritesEnabled, type HostExecRequest } from "./lib/hostExec";
import { parseHostConfirm } from "./components/HostConfirmCard";
import { AccountMenu } from "./components/AccountMenu";
import { AgentAvatar } from "./components/AgentAvatar";
import { NewChatPopover, type CreateBotInput } from "./components/NewChatPopover";
import { avatarColor } from "./components/avatarColor";
import { ChatMessage } from "./components/ChatMessage";
import { SecretPromptModal } from "./components/SecretPromptModal";
import { Composer, PendingFile } from "./components/Composer";
import { RunStatus } from "./components/RunStatus";
import {
  BotOnboardingCard,
  ONBOARDING_WELCOME,
  isOnboardingDismissed,
  onboardingStorageKey,
  setOnboardingDismissed,
  type OnboardingOption,
} from "./components/BotOnboardingCard";

type UiMessage = Message & { streaming?: boolean; attachments?: AttachmentMeta[]; agent_name?: string };
type SettingsTab =
  | "general"
  | "llm"
  | "skills"
  | "mcp"
  | "compact"
  | "routines"
  | "sandbox"
  | "machines"
  | "secrets";

const SETTINGS_TITLE: Record<SettingsTab, string> = {
  general: "通用",
  llm: "模型",
  skills: "Skills",
  mcp: "MCP",
  compact: "压缩",
  routines: "例行任务",
  sandbox: "运行环境",
  machines: "电脑",
  secrets: "密钥",
};

function accountInitials(name: string) {
  const text = name.trim();
  if (!text) return "?";
  return [...text].slice(0, 2).join("");
}

function SettingsGlyph({ id }: { id: SettingsTab }) {
  const p = {
    width: 16,
    height: 16,
    viewBox: "0 0 24 24",
    fill: "none" as const,
    stroke: "currentColor",
    strokeWidth: 1.8,
    "aria-hidden": true as const,
  };
  switch (id) {
    case "general":
      return (
        <svg {...p}>
          <circle cx="12" cy="12" r="3" />
          <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4" />
        </svg>
      );
    case "machines":
      return (
        <svg {...p}>
          <rect x="3" y="4" width="18" height="13" rx="2" />
          <path d="M8 21h8M12 17v4" />
        </svg>
      );
    case "sandbox":
      return (
        <svg {...p}>
          <path d="M4 7h16v11a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V7z" />
          <path d="M4 7l8 5 8-5M9 3h6" />
        </svg>
      );
    case "llm":
      return (
        <svg {...p}>
          <rect x="4" y="4" width="16" height="16" rx="3" />
          <path d="M9 9h6v6H9z" />
        </svg>
      );
    case "skills":
      return (
        <svg {...p}>
          <path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20" />
          <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z" />
        </svg>
      );
    case "mcp":
      return (
        <svg {...p}>
          <path d="M9 7v4a3 3 0 0 0 6 0V7" />
          <path d="M8 7h1M15 7h1M12 14v6M9 20h6" />
        </svg>
      );
    case "compact":
      return (
        <svg {...p}>
          <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
        </svg>
      );
    case "routines":
      return (
        <svg {...p}>
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v5l3 2" />
        </svg>
      );
    case "secrets":
      return (
        <svg {...p}>
          <circle cx="8" cy="15" r="4" />
          <path d="M11 13l9-9 2 2-2 1-1-1-2 2 1 1-2 2" />
        </svg>
      );
  }
}
type LastActiveSelection = { kind: "agent" | "channel"; id: string };
type ConvRunState = {
  abort: AbortController;
  generation: number;
  assistantId: string | null;
  runLabel: string;
  selectionKey: string; // `agent:${id}` | `channel:${id}` for sidebar busy
};

const lastActiveStorageKey = (userId: string) => `openbot_last_active_${userId}`;

function readLastActiveSelection(userId: string): LastActiveSelection | null {
  try {
    const raw = localStorage.getItem(lastActiveStorageKey(userId));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<LastActiveSelection>;
    if ((parsed.kind === "agent" || parsed.kind === "channel") && typeof parsed.id === "string" && parsed.id) {
      return { kind: parsed.kind, id: parsed.id };
    }
  } catch {
    // Ignore malformed or unavailable localStorage entries.
  }
  return null;
}

const emptyLLMForm: LLMInput = {
  name: "默认连接",
  base_url: "",
  api_key: "",
  model: "",
  enable_tools: false,
  is_default: true,
  context_window: null,
};


export default function App() {
  const [user, setUser] = useState<User | null>(() => getStoredUser());
  const [token, setToken] = useState<string | null>(() => getToken());
  const [authMode, setAuthMode] = useState<"login" | "register">("login");
  const [authUser, setAuthUser] = useState("");
  const [authPass, setAuthPass] = useState("");
  const [authError, setAuthError] = useState("");
  const [authBusy, setAuthBusy] = useState(false);
  const [oidcEnabled, setOidcEnabled] = useState(false);

  const [agents, setAgents] = useState<Agent[]>([]);
  const [agentId, setAgentId] = useState("");
  const [conversation, setConversation] = useState<Conversation | null>(null);
  const [messages, setMessages] = useState<UiMessage[]>([]);
  const [input, setInput] = useState("");
  const [convSearch, setConvSearch] = useState("");
  const [showScrollBottom, setShowScrollBottom] = useState(false);
  const [sending, setSending] = useState(false);
  const [taskBusy, setTaskBusy] = useState(false);
  const [taskConvIds, setTaskConvIds] = useState<Set<string>>(() => new Set());
  const taskConvIdsRef = useRef(taskConvIds);
  taskConvIdsRef.current = taskConvIds;
  const [, setStatus] = useState(""); // chat chrome status bar removed; keep setter for clear/error paths
  const [runLabel, setRunLabel] = useState("正在思考…");
  const [pendingFiles, setPendingFiles] = useState<PendingFile[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [showNewChat, setShowNewChat] = useState(false);
  const [onboardingDismissed, setOnboardingDismissedState] = useState(false);
  const newChatBtnRef = useRef<HTMLButtonElement>(null);
  const [settingsTab, setSettingsTab] = useState<SettingsTab>("general");
  const [emailCopied, setEmailCopied] = useState(false);

  const [llms, setLlms] = useState<LLMConnection[]>([]);
  const [llmForm, setLLMForm] = useState<LLMInput>(emptyLLMForm);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [llmBusy, setLLMBusy] = useState(false);
  const [llmMsg, setLLMMsg] = useState("");

  const [agentBusy, setAgentBusy] = useState(false);
  const [channelBusy, setChannelBusy] = useState(false);

  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillsMsg, setSkillsMsg] = useState("");
  const [skillsBusy, setSkillsBusy] = useState(false);
  const [skillName, setSkillName] = useState("");
  const [skillDesc, setSkillDesc] = useState("");
  const [skillBody, setSkillBody] = useState("");

  const [compactCfg, setCompactCfg] = useState<CompactConfig | null>(null);
  const confirm = useConfirm();
  const [channels, setChannels] = useState<Channel[]>([]);
  const [mcpServers, setMcpServers] = useState<MCPServer[]>([]);
  const [mcpMsg, setMcpMsg] = useState("");
  const [mcpBusy, setMcpBusy] = useState(false);
  const [mcpForm, setMcpForm] = useState<MCPServerInput>({
    name: "echo",
    transport: "stdio",
    command: "",
    args: [],
    url: "",
    enabled: true,
  });
  const [mcpArgsText, setMcpArgsText] = useState("[]");
  const [mcpTestResult, setMcpTestResult] = useState("");
  const [mcpCallServerId, setMcpCallServerId] = useState("");
  const [mcpCallToolName, setMcpCallToolName] = useState("echo");
  const [mcpCallArgs, setMcpCallArgs] = useState('{"message":"hello"}');
  const [mcpCallResult, setMcpCallResult] = useState("");

  const [routines, setRoutines] = useState<Routine[]>([]);
  const [routinesMsg, setRoutinesMsg] = useState("");
  const [routinesBusy, setRoutinesBusy] = useState(false);
  const [routineForm, setRoutineForm] = useState<RoutineInput>({
    name: "",
    prompt: "",
    schedule_cron: "0 9 * * *",
    enabled: true,
  });

  const [sandbox, setSandbox] = useState<Sandbox | null>(null);
  const [sandboxMsg, setSandboxMsg] = useState("");
  const [sandboxBusy, setSandboxBusy] = useState(false);
  const [sandboxCmd, setSandboxCmd] = useState("echo hi");
  const [sandboxExecOut, setSandboxExecOut] = useState("");
  const [sandboxPath, setSandboxPath] = useState("/workspace");
  const [sandboxLsOut, setSandboxLsOut] = useState("");
  const [sandboxFilePath, setSandboxFilePath] = useState("hello.txt");
  const [sandboxFileContent, setSandboxFileContent] = useState("hello from open-bot");
  const [machines, setMachines] = useState<Machine[]>([]);
  const [machinesMsg, setMachinesMsg] = useState("");
  const [machinesBusy, setMachinesBusy] = useState(false);
  const [hostMachineId, setHostMachineId] = useState<string | null>(() => getStoredMachineId());
  const [hostWritesOn, setHostWritesOn] = useState(() => hostWritesEnabled());
  const [hostActivity, setHostActivity] = useState("");
  const hostConfirmResolvers = useRef(new Map<string, (ok: boolean) => void>());
  const askHostConfirmRef = useRef<(req: HostExecRequest) => Promise<boolean>>(async () => false);
  const applyRemoteHostDecisionRef = useRef<(msg: Message) => void>(() => {});
  const clientEnv = useMemo(() => detectClientContext(), []);
  const [botSecrets, setBotSecrets] = useState<BotSecretMeta[]>([]);
  const [secretRequests, setSecretRequests] = useState<BotSecretRequest[]>([]);
  const [secretPrompt, setSecretPrompt] = useState<BotSecretRequest | null>(null);
  const [secretFormName, setSecretFormName] = useState("api_token");
  const [secretFormValue, setSecretFormValue] = useState("");
  const [secretFormOrigin, setSecretFormOrigin] = useState("https://api.github.com");
  const [secretMsg, setSecretMsg] = useState("");

  /** Per-conversation in-flight SSE runs — switching bots must not abort these. */
  const runsRef = useRef<Map<string, ConvRunState>>(new Map());
  const messagesByConvRef = useRef<Map<string, UiMessage[]>>(new Map());
  /** Mirrors `messages` so switch-away can snapshot without waiting a render. */
  const messagesLiveRef = useRef<UiMessage[]>([]);
  const conversationRef = useRef<Conversation | null>(null);
  const didRestoreLastActive = useRef<string | null>(null);
  /** Ignores stale openPrimaryConversation / openChannel results after rapid switches. */
  const selectGenRef = useRef(0);
  /** Tick so sidebar busy indicators re-render when runs start/end. */
  const [runBusyTick, setRunBusyTick] = useState(0);

  const authed = Boolean(token && user);

  const refreshMachines = useCallback(async () => {
    const list = await listMachines();
    setMachines(list);
  }, []);

  // Desktop/mobile: register this host + heartbeat while authenticated.
  useEffect(() => {
    if (!authed) return;
    if (!shouldRegisterAsHost(clientEnv)) return;
    let cancelled = false;
    let timer: ReturnType<typeof setInterval> | null = null;
    const run = async () => {
      try {
        const m = await registerMachine({
          machine_key: getOrCreateMachineKey(),
          label: defaultMachineLabel(clientEnv),
          platform: clientEnv.platform,
          os: clientEnv.os,
          arch: clientEnv.arch,
          app: clientEnv.app,
          app_version: clientEnv.app_version,
        });
        if (cancelled) return;
        setStoredMachineId(m.id);
        setHostMachineId(m.id);
        setMachines((prev) => {
          const others = prev.filter((x) => x.id !== m.id);
          return [m, ...others];
        });
        timer = setInterval(() => {
          const id = getStoredMachineId();
          if (!id) return;
          void heartbeatMachine(id).catch(() => {
            /* ignore transient */
          });
        }, 30_000);
      } catch (err) {
        if (!cancelled) {
          console.warn("machine register failed", err);
        }
      }
    };
    void run();
    return () => {
      cancelled = true;
      if (timer) clearInterval(timer);
    };
  }, [authed, clientEnv]);

  useEffect(() => {
    if (!authed || !token || clientEnv.app !== "tauri" || !hostMachineId) return;
    const stop = startHostExecSession({
      token,
      machineId: hostMachineId,
      confirm: (req) => askHostConfirmRef.current(req),
    });
    return () => {
      stop();
      for (const resolve of hostConfirmResolvers.current.values()) resolve(false);
      hostConfirmResolvers.current.clear();
    };
  }, [authed, token, clientEnv.app, hostMachineId]);


  const refreshLLMs = useCallback(async () => {
    const list = await listLLMConnections();
    setLlms(list);
  }, []);

  const refreshAgents = useCallback(async (): Promise<Agent[]> => {
    const list = await listAgents();
    setAgents(list);
    setAgentId((prev) => (list.some((a) => a.id === prev) ? prev : list[0]?.id ?? ""));
    return list;
  }, []);

  const refreshSkills = useCallback(async () => {
    const list = await listSkills();
    setSkills(list);
  }, []);

  const refreshChannels = useCallback(async () => {
    const chs = await listChannels();
    setChannels(chs);
    return chs;
  }, []);

  const refreshCompact = useCallback(async () => {
    const cfg = await fetchCompactConfig();
    setCompactCfg(cfg);
  }, []);

  const refreshConversations = useCallback(async () => {
    // Sidebar is bot-first; refresh agent previews (last_message) instead of listing sessions.
    await refreshAgents();
  }, [refreshAgents]);

  useEffect(() => {
    conversationRef.current = conversation;
  }, [conversation]);

  useEffect(() => {
    messagesLiveRef.current = messages;
  }, [messages]);

  useEffect(() => {
    if (!token) return;
    let stopped = false;
    let socket: WebSocket | null = null;
    let retry: number | undefined;
    const pullOpen = () => {
      const id = conversationRef.current?.id;
      if (!id) return;
      void listMessagesWithStatus(id)
        .then((listed) => {
          if (conversationRef.current?.id !== id) return;
          patchConvMessages(id, (prev) => {
            let next = prev;
            for (const m of listed.messages) {
              if (m.role === "summary") continue;
              applyRemoteHostDecisionRef.current(m);
              next = mergeIncomingMessage(next, m);
            }
            return next;
          });
          noteTask(id, Boolean(listed.task_active));
        })
        .catch(() => {});
    };
    const connect = () => {
      if (stopped) return;
      const ws = new WebSocket(chatEventsWebSocketUrl(token));
      socket = ws;
      ws.onopen = () => {
        void refreshAgents().catch(() => {});
        pullOpen();
      };
      ws.onmessage = (ev) => {
        let data: { type?: string; message?: Message; conversation_id?: string; status?: string; label?: string };
        try {
          data = JSON.parse(String(ev.data));
        } catch {
          return;
        }
        if (data.type === "conversation_message" && data.message) {
          acceptServerMessage(data.message);
          void refreshAgents().catch(() => {});
          return;
        }
        if (data.type === "task_status" && data.conversation_id) {
          const active = data.status === "running" || data.status === "queued";
          noteTask(data.conversation_id, active, data.label);
          void refreshAgents().catch(() => {});
          return;
        }
        if (data.type === "host_activity") {
          const row = data as { active?: boolean; label?: string };
          setHostActivity(row.active && row.label ? row.label : "");
        }
      };
      ws.onclose = () => {
        if (stopped) return;
        retry = window.setTimeout(connect, 2000);
      };
    };
    connect();
    const onVisible = () => {
      if (document.visibilityState === "visible") {
        void refreshAgents().catch(() => {});
        pullOpen();
      }
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      stopped = true;
      if (retry) window.clearTimeout(retry);
      document.removeEventListener("visibilitychange", onVisible);
      socket?.close();
    };
  }, [token, refreshAgents]);

  useEffect(() => {
    const id = conversation?.id;
    if (!token || !id || !taskConvIds.has(id)) return;
    const timer = window.setInterval(() => {
      void listMessagesWithStatus(id)
        .then((listed) => {
          if (conversationRef.current?.id !== id) return;
          patchConvMessages(id, (prev) => {
            let next = prev;
            for (const m of listed.messages) {
              if (m.role === "summary") continue;
              applyRemoteHostDecisionRef.current(m);
              next = mergeIncomingMessage(next, m);
            }
            return next;
          });
          noteTask(id, Boolean(listed.task_active));
        })
        .catch(() => {});
    }, 4000);
    return () => window.clearInterval(timer);
  }, [token, conversation?.id, taskConvIds]);

  const busySelectionKeys = useMemo(() => {
    void runBusyTick;
    const keys = new Set<string>();
    for (const run of runsRef.current.values()) {
      keys.add(run.selectionKey);
    }
    for (const a of agents) {
      if (a.task_active || (a.conversation_id && taskConvIds.has(a.conversation_id))) {
        keys.add(`agent:${a.id}`);
      }
    }
    for (const ch of channels) {
      if (ch.task_active || (ch.conversation_id && taskConvIds.has(ch.conversation_id))) {
        keys.add(`channel:${ch.id}`);
      }
    }
    return keys;
  }, [runBusyTick, agents, channels, taskConvIds]);

  const messagesRef = useRef<HTMLDivElement | null>(null);
  /** When true, keep the message list pinned to the latest content. */
  const stickToBottomRef = useRef(true);

  const isNearBottom = useCallback(() => {
    const el = messagesRef.current;
    if (!el) return true;
    return el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }, []);

  const scrollToBottom = useCallback((smooth = false) => {
    const el = messagesRef.current;
    if (!el) return;
    // Scroll the messages pane only — avoid scrollIntoView (can move page/sidebar).
    if (smooth) {
      el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
    } else {
      el.scrollTop = el.scrollHeight;
    }
    stickToBottomRef.current = true;
    setShowScrollBottom(false);
  }, []);

  const onMessagesScroll = useCallback(() => {
    const near = isNearBottom();
    stickToBottomRef.current = near;
    setShowScrollBottom(!near);
  }, [isNearBottom]);

  // Keep pinned while streaming / status updates unless the user scrolled up.
  useEffect(() => {
    if (!stickToBottomRef.current) {
      setShowScrollBottom(true);
      return;
    }
    const id = requestAnimationFrame(() => {
      scrollToBottom(false);
    });
    return () => cancelAnimationFrame(id);
  }, [messages, sending, runLabel, scrollToBottom]);




  const activeAgent = useMemo(
    () => agents.find((a) => a.id === agentId) ?? agents[0],
    [agents, agentId],
  );

  const onboardingKey = useMemo(
    () => onboardingStorageKey(conversation?.id, agentId),
    [conversation?.id, agentId],
  );

  useEffect(() => {
    setOnboardingDismissedState(isOnboardingDismissed(onboardingKey));
  }, [onboardingKey]);

  const hasChatMessages = useMemo(
    () => messages.some((m) => m.role === "user" || m.role === "assistant"),
    [messages],
  );

  const showOnboarding = Boolean(authed && !hasChatMessages && !onboardingDismissed && !sending);


  const filteredAgents = useMemo(() => {
    const q = convSearch.trim().toLowerCase();
    if (!q) return agents;
    return agents.filter((a) => {
      const hay = `${a.name} ${a.description || ""} ${a.id} ${a.last_message || ""}`.toLowerCase();
      return hay.includes(q);
    });
  }, [agents, convSearch]);

  const agentNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agents) m.set(a.id, a.name);
    return m;
  }, [agents]);

  const filteredChannels = useMemo(() => {
    const q = convSearch.trim().toLowerCase();
    if (!q) return channels;
    return channels.filter((ch) => {
      const memberNames = (ch.members || []).map((id) => agentNameById.get(id) || id).join(" ");
      const hay = `${ch.name} ${memberNames}`.toLowerCase();
      return hay.includes(q);
    });
  }, [channels, convSearch, agentNameById]);

  const activeChannel = useMemo(() => {
    if (!conversation?.channel_id) return null;
    return channels.find((c) => c.id === conversation.channel_id) ?? null;
  }, [channels, conversation?.channel_id]);

  const groupMentionMembers = useMemo(() => {
    if (!activeChannel?.members?.length) return undefined;
    return activeChannel.members
      .map((id) => agents.find((a) => a.id === id))
      .filter((a): a is Agent => Boolean(a));
  }, [activeChannel, agents]);

  const defaultLLM = useMemo(
    () => llms.find((c) => c.is_default) ?? llms[0],
    [llms],
  );

  useEffect(() => {
    let cancelled = false;
    fetchOIDCConfig()
      .then((c) => {
        if (!cancelled) setOidcEnabled(Boolean(c.enabled));
      })
      .catch(() => {
        if (!cancelled) setOidcEnabled(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const onCasdoorLogin = async () => {
    setAuthBusy(true);
    setAuthError("");
    try {
      const { authorize_url } = await startOIDCLogin();
      window.location.href = authorize_url;
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : String(err));
      setAuthBusy(false);
    }
  };

  const onAuth = async (e: FormEvent) => {
    e.preventDefault();
    setAuthBusy(true);
    setAuthError("");
    try {
      const fn = authMode === "login" ? login : register;
      const res = await fn(authUser.trim(), authPass);
      setSession(res.token, res.user);
      setToken(res.token);
      setUser(res.user);
      setAuthPass("");
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : String(err));
    } finally {
      setAuthBusy(false);
    }
  };

  const saveLastActiveSelection = useCallback((selection: LastActiveSelection) => {
    if (!user?.id) return;
    try {
      localStorage.setItem(lastActiveStorageKey(user.id), JSON.stringify(selection));
    } catch {
      // Ignore unavailable localStorage.
    }
  }, [user?.id]);

  const onLogout = () => {
    didRestoreLastActive.current = null;
    for (const [convId, run] of [...runsRef.current.entries()]) {
      run.generation += 1;
      run.abort.abort();
      void cancelConversationRun(convId).catch(() => {});
    }
    runsRef.current.clear();
    messagesByConvRef.current.clear();
    clearSession();
    setToken(null);
    setUser(null);
    conversationRef.current = null;
    setConversation(null);
    setMessages([]);
    setSending(false);
    setTaskBusy(false);
    setTaskConvIds(new Set());
    setLlms([]);
    setStatus("");
    setRunBusyTick((n) => n + 1);
  };

  const ensureConversation = useCallback(async () => {
    if (conversation?.channel_id) return conversation;
    if (conversation && conversation.agent_id === agentId && !conversation.channel_id) {
      return conversation;
    }
    if (!agentId) {
      throw new Error("请先创建助手");
    }
    const conv = await openPrimaryConversation(agentId);
    conversationRef.current = conv;
    setConversation(conv);
    return conv;
  }, [agentId, conversation]);

  const bumpRunBusy = () => setRunBusyTick((n) => n + 1);

  const selectionKeyForConv = (conv: Conversation) =>
    conv.channel_id ? `channel:${conv.channel_id}` : `agent:${conv.agent_id}`;

  /** Update message buffer for a conversation; sync React state only if that conv is viewed. */
  const patchConvMessages = (convId: string, updater: (prev: UiMessage[]) => UiMessage[]) => {
    const prev = messagesByConvRef.current.get(convId) ?? [];
    const next = updater(prev);
    messagesByConvRef.current.set(convId, next);
    if (conversationRef.current?.id === convId) {
      messagesLiveRef.current = next;
      setMessages(next);
    }
  };

  const noteTask = (convId: string, active: boolean, label?: string) => {
    const cur = taskConvIdsRef.current;
    const has = cur.has(convId);
    if (active !== has) {
      const next = new Set(cur);
      if (active) next.add(convId);
      else next.delete(convId);
      taskConvIdsRef.current = next;
      setTaskConvIds(next);
    }
    if (conversationRef.current?.id === convId) {
      setTaskBusy(active);
      if (active && label) setRunLabel(label);
      if (!active) setRunLabel("正在思考…");
    }
  };

  const releaseHostConfirm = (reqId: string, ok: boolean) => {
    const resolve = hostConfirmResolvers.current.get(reqId);
    if (!resolve) return;
    hostConfirmResolvers.current.delete(reqId);
    resolve(ok);
  };

  const applyRemoteHostDecision = (msg: Message) => {
    if (msg.role !== "host_confirm") return;
    const parsed = parseHostConfirm(msg.content);
    if (!parsed || (parsed.status !== "allowed" && parsed.status !== "denied")) return;
    releaseHostConfirm(parsed.req_id, parsed.status === "allowed");
  };
  applyRemoteHostDecisionRef.current = applyRemoteHostDecision;

  const acceptServerMessage = (msg: Message) => {
    applyRemoteHostDecision(msg);
    const convId = msg.conversation_id;
    if (!convId) return;
    patchConvMessages(convId, (prev) => mergeIncomingMessage(prev, msg));
  };

  askHostConfirmRef.current = (req) => {
    const conversationId = req.conversation_id || conversationRef.current?.id || "";
    return new Promise<boolean>((resolve) => {
      hostConfirmResolvers.current.set(req.req_id, resolve);
      if (!conversationId) return;
      const raw = req.content || "";
      let preview = raw.length > 180 ? `${raw.slice(0, 180)}…` : raw;
      if (req.op === "shell" && req.dest === "terminal" && !preview) {
        preview = "会打开终端窗口，你可以在里面输入密码或继续操作";
      }
      void createHostConfirm(conversationId, {
        req_id: req.req_id,
        op: req.op,
        path: req.path || "",
        dest: req.dest || "",
        preview,
      })
        .then((msg) => acceptServerMessage(msg))
        .catch(() => {
          acceptServerMessage({
            id: `local-${req.req_id}`,
            role: "host_confirm",
            conversation_id: conversationId,
            created_at: new Date().toISOString(),
            content: JSON.stringify({
              req_id: req.req_id,
              op: req.op,
              path: req.path || "",
              dest: req.dest || "",
              preview,
              status: "pending",
            }),
          });
        });
    });
  };

  const settleHostConfirm = (message: Message, ok: boolean) => {
    const parsed = parseHostConfirm(message.content);
    if (!parsed?.req_id || parsed.status === "allowed" || parsed.status === "denied") return;
    const status = ok ? "allowed" : "denied";
    const content = JSON.stringify({ ...parsed, status });
    const convId = message.conversation_id || conversationRef.current?.id || "";
    if (convId) {
      patchConvMessages(convId, (prev) => prev.map((m) => (m.id === message.id ? { ...m, content } : m)));
      if (!message.id.startsWith("local-")) {
        void decideHostConfirm(convId, message.id, status)
          .then((saved) => acceptServerMessage(saved))
          .catch(() => {});
      }
    }
    releaseHostConfirm(parsed.req_id, ok);
  };

  const stampSavedMessage = (convId: string, messageId: string) => {
    if (!messageId) return;
    const run = runsRef.current.get(convId);
    const localId = run?.assistantId;
    patchConvMessages(convId, (prev) => {
      if (prev.some((m) => m.id === messageId)) return prev;
      if (!localId) return prev;
      return prev.map((m) => (m.id === localId ? { ...m, id: messageId } : m));
    });
    if (run && localId && run.assistantId === localId) {
      run.assistantId = messageId;
    }
  };

  const snapshotViewedMessages = () => {
    const id = conversationRef.current?.id;
    if (!id) return;
    messagesByConvRef.current.set(id, messagesLiveRef.current);
  };

  const sealStreamingMessagesForConv = (convId: string, markStopped: boolean) => {
    patchConvMessages(convId, (prev) =>
      prev.map((m) => {
        if (!m.streaming) return m;
        const content =
          m.content && m.content.trim()
            ? m.content
            : markStopped
              ? "（已停止）"
              : m.content;
        return { ...m, streaming: false, content };
      }),
    );
    const run = runsRef.current.get(convId);
    if (run) run.assistantId = null;
  };

  /** Stop a specific conversation's run (default: currently viewed). Does not touch other bots. */
  const stopCurrentRun = async (opts?: {
    markStopped?: boolean;
    waitForFlush?: boolean;
    conversationId?: string;
  }) => {
    const convId = opts?.conversationId ?? conversationRef.current?.id;
    if (!convId) return;
    const run = runsRef.current.get(convId);
    if (run) {
      run.generation += 1;
      run.abort.abort();
      runsRef.current.delete(convId);
      bumpRunBusy();
    }
    sealStreamingMessagesForConv(convId, opts?.markStopped !== false);
    if (conversationRef.current?.id === convId) {
      setSending(false);
      setRunLabel("正在思考…");
    }
    const cancelPromise = cancelConversationRun(convId).catch(() => {});
    if (opts?.waitForFlush) {
      await cancelPromise;
    }
  };

  const mapApiMessages = (msgs: Message[], nameLookup: Map<string, string> | Agent[]) => {
    const nameOf = (agentId: string) => {
      if (Array.isArray(nameLookup)) {
        return nameLookup.find((a) => a.id === agentId)?.name || agentId;
      }
      return nameLookup.get(agentId) || agentId;
    };
    return msgs
      .filter((m) => m.role !== "summary")
      .map((m) => ({
        ...m,
        agent_name: m.agent_id ? nameOf(m.agent_id) : undefined,
      }));
  };

  /** After refresh / reconnect: rejoin a server-owned in-flight run for this conversation. */
  const resumeActiveRun = async (
    conv: Conversation,
    nameLookup: Map<string, string> | Agent[],
  ) => {
    if (runsRef.current.has(conv.id)) return;
    let active = false;
    try {
      active = (await getConversationRunStatus(conv.id)).active;
    } catch {
      return;
    }
    if (!active) return;

    const ac = new AbortController();
    const gen = (runsRef.current.get(conv.id)?.generation ?? 0) + 1;
    runsRef.current.set(conv.id, {
      abort: ac,
      generation: gen,
      assistantId: null,
      runLabel: "正在思考…",
      selectionKey: selectionKeyForConv(conv),
    });
    bumpRunBusy();
    if (conversationRef.current?.id === conv.id) {
      setSending(true);
      setRunLabel("正在思考…");
    }

    const isRunCurrent = () => {
      const run = runsRef.current.get(conv.id);
      return Boolean(run && run.generation === gen);
    };
    const isViewingStream = () => conversationRef.current?.id === conv.id;
    const setRunLabelForStream = (label: string) => {
      const run = runsRef.current.get(conv.id);
      if (run && run.generation === gen) run.runLabel = label;
      if (isViewingStream()) setRunLabel(label);
    };
    const ensureStreamingBubble = (agentId?: string, agentName?: string) => {
      const run = runsRef.current.get(conv.id);
      if (!run || run.generation !== gen) return;
      if (run.assistantId) {
        const cur = (messagesByConvRef.current.get(conv.id) ?? []).find((m) => m.id === run.assistantId);
        if (cur?.streaming) {
          if (agentId && !cur.agent_id) {
            patchConvMessages(conv.id, (prev) =>
              prev.map((m) =>
                m.id === run.assistantId
                  ? { ...m, agent_id: agentId, agent_name: agentName || m.agent_name, streaming: true }
                  : m,
              ),
            );
          }
          return;
        }
      }
      const nextId = `local-asst-resume-${Date.now()}`;
      run.assistantId = nextId;
      patchConvMessages(conv.id, (prev) => [
        ...prev.filter((m) => !m.streaming),
        {
          id: nextId,
          role: "assistant" as const,
          content: "",
          streaming: true,
          agent_id: agentId,
          agent_name: agentName,
        },
      ]);
    };

    try {
      await subscribeConversationEvents(
        conv.id,
        {
          onAgentStart: (info) => {
            if (!isRunCurrent()) return;
            const name =
              info.agent_name ||
              (Array.isArray(nameLookup)
                ? nameLookup.find((a) => a.id === info.agent_id)?.name
                : nameLookup.get(info.agent_id)) ||
              info.agent_id;
            setRunLabelForStream(`${name} 正在回复…`);
            ensureStreamingBubble(info.agent_id, name);
          },
          onToken: (text) => {
            if (!isRunCurrent()) return;
            ensureStreamingBubble();
            const curId = runsRef.current.get(conv.id)?.assistantId;
            if (!curId) return;
            patchConvMessages(conv.id, (prev) =>
              prev.map((m) => (m.id === curId ? { ...m, content: m.content + text, streaming: true } : m)),
            );
          },
          onMeta: (meta) => {
            if (!isRunCurrent()) return;
            if (meta.phase === "resumed") {
              if (isViewingStream()) {
                setSending(true);
                setRunLabel("正在思考…");
              }
              return;
            }
            if (meta.phase === "cancelled") {
              sealStreamingMessagesForConv(conv.id, true);
              if (isViewingStream()) {
                setSending(false);
                setRunLabel("正在思考…");
              }
            }
            if (meta.phase === "message_saved" && typeof meta.message_id === "string") {
              stampSavedMessage(conv.id, meta.message_id);
            }
            if (meta.phase === "task_queued") {
              noteTask(conv.id, true, "正在做，做好会发在这里");
            }
          },
          onStatus: (data) => {
            if (!isRunCurrent()) return;
            const phase = String(data.phase || "");
            const tool = typeof data.tool === "string" ? data.tool : "";
            if (typeof data.label === "string" && data.label.trim()) {
              if (phase === "tool" && tool) {
                setRunLabelForStream(`${data.label} · ${tool}`);
              } else {
                setRunLabelForStream(data.label);
              }
              return;
            }
            if (phase === "tool") {
              setRunLabelForStream(tool ? `正在运行命令 · ${tool}` : "正在运行命令");
            } else if (phase === "thinking" || phase === "tool_done") {
              setRunLabelForStream("正在思考…");
            }
          },
          onError: (msg) => {
            if (!isRunCurrent()) return;
            ensureStreamingBubble();
            const curId = runsRef.current.get(conv.id)?.assistantId;
            patchConvMessages(conv.id, (prev) =>
              prev.map((m) =>
                m.id === curId
                  ? { ...m, streaming: false, content: m.content || `（失败）${msg}` }
                  : m,
              ),
            );
          },
          onDone: () => {
            if (!isRunCurrent()) return;
            sealStreamingMessagesForConv(conv.id, false);
            setRunLabelForStream("正在思考…");
            void listMessages(conv.id)
              .then((msgs) => {
                if (!isRunCurrent() && runsRef.current.has(conv.id)) return;
                // Only replace if this resume gen already cleaned or still current after seal.
                if (conversationRef.current?.id === conv.id || messagesByConvRef.current.has(conv.id)) {
                  const mapped = mapApiMessages(msgs, nameLookup);
                  messagesByConvRef.current.set(conv.id, mapped);
                  if (conversationRef.current?.id === conv.id) {
                    messagesLiveRef.current = mapped;
                    setMessages(mapped);
                  }
                }
              })
              .catch(() => {});
            void refreshConversations();
          },
        },
        ac.signal,
      );
    } catch (err) {
      const aborted =
        (err instanceof DOMException && err.name === "AbortError") ||
        (err instanceof Error && err.name === "AbortError");
      if (aborted) return;
      if (!isRunCurrent()) return;
      console.warn("resume run failed", err);
    } finally {
      const run = runsRef.current.get(conv.id);
      if (run && run.generation === gen) {
        runsRef.current.delete(conv.id);
        bumpRunBusy();
        if (conversationRef.current?.id === conv.id) {
          setSending(false);
        }
      }
    }
  };


  const applyConversationView = (conv: Conversation, apiMsgs?: Message[], nameLookup?: Map<string, string> | Agent[]) => {
    stickToBottomRef.current = true;
    // Sync ref immediately so in-flight tokens for the previous conv cannot paint into this view.
    conversationRef.current = conv;
    setConversation(conv);
    const activeRun = runsRef.current.get(conv.id);
    if (activeRun) {
      const buffered = messagesByConvRef.current.get(conv.id) ?? [];
      messagesLiveRef.current = buffered;
      setMessages(buffered);
      setSending(true);
      setRunLabel(activeRun.runLabel || "正在思考…");
      return;
    }
    setSending(false);
    setRunLabel("正在思考…");
    if (apiMsgs) {
      const mapped = mapApiMessages(apiMsgs, nameLookup ?? agents);
      messagesByConvRef.current.set(conv.id, mapped);
      messagesLiveRef.current = mapped;
      setMessages(mapped);
    }
  };

  const onSelectAgent = useCallback((id: string) => {
    void (async () => {
      snapshotViewedMessages();
      const selectGen = ++selectGenRef.current;
      // Hide Stop for the previous run while the target chat loads; do not cancel it.
      setSending(false);
      setRunLabel("正在思考…");
      setAgentId(id);
      setPendingFiles([]);
      setStatus("");
      try {
        const conv = await openPrimaryConversation(id);
        if (selectGen !== selectGenRef.current) return;
        conversationRef.current = conv;
        const activeRun = runsRef.current.get(conv.id);
        let apiMsgs: Message[] | undefined;
        let runActive = false;
        if (!activeRun) {
          const listed = await listMessagesWithStatus(conv.id);
          if (selectGen !== selectGenRef.current) return;
          apiMsgs = listed.messages;
          runActive = Boolean(listed.run_active);
          noteTask(conv.id, Boolean(listed.task_active), listed.task_active ? "正在做，做好会发在这里" : undefined);
        } else {
          setTaskBusy(taskConvIdsRef.current.has(conv.id));
        }
        applyConversationView(conv, apiMsgs, agents);
        saveLastActiveSelection({ kind: "agent", id });
        void refreshAgents().catch(() => {});
        if (!activeRun && runActive) {
          void resumeActiveRun(conv, agents);
        }
      } catch (err) {
        if (selectGen !== selectGenRef.current) return;
        conversationRef.current = null;
        setConversation(null);
        setMessages([]);
        setSending(false);
        setStatus(err instanceof Error ? err.message : String(err));
      }
    })();
  }, [agents, refreshAgents, saveLastActiveSelection]);

  const openChannelChat = useCallback(async (channelId: string) => {
    try {
      snapshotViewedMessages();
      const selectGen = ++selectGenRef.current;
      setSending(false);
      setRunLabel("正在思考…");
      const { conversation: conv } = await openChannelConversation(channelId);
      if (selectGen !== selectGenRef.current) return;
      conversationRef.current = conv;
      setAgentId(conv.agent_id);
      setPendingFiles([]);
      setStatus("");
      const activeRun = runsRef.current.get(conv.id);
      let apiMsgs: Message[] | undefined;
      let runActive = false;
      if (!activeRun) {
        const listed = await listMessagesWithStatus(conv.id);
        if (selectGen !== selectGenRef.current) return;
        apiMsgs = listed.messages;
        runActive = Boolean(listed.run_active);
        noteTask(conv.id, Boolean(listed.task_active), listed.task_active ? "正在做，做好会发在这里" : undefined);
      } else {
        setTaskBusy(taskConvIdsRef.current.has(conv.id));
      }
      applyConversationView(conv, apiMsgs, agentNameById);
      saveLastActiveSelection({ kind: "channel", id: channelId });
      await refreshChannels();
      if (!activeRun && runActive) {
        void resumeActiveRun(conv, agentNameById);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    }
  }, [agentNameById, refreshChannels, saveLastActiveSelection]);

  const sendUserText = async (rawContent: string, filesToSend: PendingFile[] = []) => {
    const content = rawContent.trim();
    if ((!content && filesToSend.length === 0) || !authed) return;
    // keep-partial-next-turn: only interrupt the *current* conversation's run.
    const viewedId = conversationRef.current?.id;
    if (viewedId && runsRef.current.has(viewedId)) {
      await stopCurrentRun({ markStopped: true, waitForFlush: true, conversationId: viewedId });
    }
    setSending(true);
    setInput("");
    setPendingFiles([]);
    setStatus("");
    setRunLabel("正在思考…");
    // Choosing / answering onboarding hides the card for this conversation
    setOnboardingDismissed(onboardingKey);
    setOnboardingDismissedState(true);
    // After send always jump to bottom (common chat pattern).
    stickToBottomRef.current = true;
    setShowScrollBottom(false);
    const localAttachments: AttachmentMeta[] = filesToSend.map((f, i) => ({
      id: `local-${i}`,
      name: f.file.name,
      mime: f.file.type || "application/octet-stream",
      size: f.file.size,
      path: "",
    }));
    const userMsg: UiMessage = {
      id: `local-user-${Date.now()}`,
      role: "user",
      content: content || (filesToSend.length ? `（${filesToSend.length} 个附件）` : ""),
      attachments: localAttachments.length ? localAttachments : undefined,
    };
    const assistantId = `local-asst-${Date.now()}`;
    const appendOptimistic = (prev: UiMessage[]) => [
      ...prev,
      userMsg,
      { id: assistantId, role: "assistant" as const, content: "", streaming: true },
    ];
    if (viewedId) {
      patchConvMessages(viewedId, appendOptimistic);
    } else {
      const next = appendOptimistic(messagesLiveRef.current);
      messagesLiveRef.current = next;
      setMessages(next);
    }
    const mentionAgentIds = (() => {
      if (!conversation?.channel_id || !groupMentionMembers?.length) return undefined as string[] | undefined;
      const re = /@([^\s@]+)/g;
      const ids: string[] = [];
      const seen = new Set<string>();
      let m: RegExpExecArray | null;
      while ((m = re.exec(content)) !== null) {
        const tok = m[1].replace(/[.,!?;:，。！？；：]+$/u, "");
        const lower = tok.toLowerCase();
        const hit = groupMentionMembers.find(
          (a) =>
            a.id.toLowerCase() === lower ||
            a.name.toLowerCase() === lower ||
            a.name.toLowerCase().includes(lower) ||
            a.id.toLowerCase().startsWith(lower),
        );
        if (hit && !seen.has(hit.id)) {
          seen.add(hit.id);
          ids.push(hit.id);
        }
      }
      return ids.length ? ids : undefined;
    })();
    let sendSelection: LastActiveSelection | null = null;
    let sendHadError = false;
    let streamConvId: string | null = null;
    let gen = 0;
    try {
      const conv = await ensureConversation();
      streamConvId = conv.id;
      sendSelection = conv.channel_id
        ? { kind: "channel", id: conv.channel_id }
        : { kind: "agent", id: conv.agent_id };
      // Ensure buffer has optimistic messages under the real conv id (first send may create it).
      if (!messagesByConvRef.current.has(streamConvId) || conversationRef.current?.id === streamConvId) {
        messagesByConvRef.current.set(streamConvId, messagesLiveRef.current);
      }
      void refreshConversations();
      void refreshAgents().catch(() => {});
      let uploaded: AttachmentMeta[] = [];
      if (filesToSend.length > 0) {
        uploaded = [];
        for (const pf of filesToSend) {
          uploaded.push(await uploadConversationAttachment(conv.id, pf.file));
        }
        patchConvMessages(streamConvId, (prev) =>
          prev.map((m) => (m.id === userMsg.id ? { ...m, attachments: uploaded } : m)),
        );
      }
      const ac = new AbortController();
      gen = (runsRef.current.get(streamConvId)?.generation ?? 0) + 1;
      runsRef.current.set(streamConvId, {
        abort: ac,
        generation: gen,
        assistantId,
        runLabel: "正在思考…",
        selectionKey: selectionKeyForConv(conv),
      });
      bumpRunBusy();
      const isRunCurrent = () => {
        const run = runsRef.current.get(streamConvId!);
        return Boolean(run && run.generation === gen);
      };
      const isViewingStream = () => conversationRef.current?.id === streamConvId;
      const setRunLabelForStream = (label: string) => {
        const run = runsRef.current.get(streamConvId!);
        if (run && run.generation === gen) run.runLabel = label;
        if (isViewingStream()) setRunLabel(label);
      };
      await sendMessageStream(
        conv.id,
        content,
        {
          onAgentStart: (info) => {
            if (!isRunCurrent()) return;
            const name =
              info.agent_name ||
              agents.find((a) => a.id === info.agent_id)?.name ||
              info.agent_id;
            setRunLabelForStream(`${name} 正在回复…`);
            patchConvMessages(streamConvId!, (prev) => {
              const run = runsRef.current.get(streamConvId!);
              const curId = run?.assistantId;
              const cur = curId ? prev.find((m) => m.id === curId) : undefined;
              // First agent: stamp identity on the placeholder bubble.
              if (cur && !cur.content && !cur.agent_id) {
                return prev.map((m) =>
                  m.id === curId
                    ? { ...m, agent_id: info.agent_id, agent_name: name, streaming: true }
                    : m,
                );
              }
              // Subsequent agents: seal previous bubble and open a new one.
              const sealed = prev.map((m) =>
                m.id === curId ? { ...m, streaming: false } : m,
              );
              const nextId = `local-asst-${Date.now()}-${info.index ?? 0}`;
              if (run) run.assistantId = nextId;
              return [
                ...sealed,
                {
                  id: nextId,
                  role: "assistant",
                  content: "",
                  streaming: true,
                  agent_id: info.agent_id,
                  agent_name: name,
                },
              ];
            });
          },
          onToken: (text) => {
            if (!isRunCurrent()) return;
            const curId = runsRef.current.get(streamConvId!)?.assistantId;
            if (!curId) return;
            patchConvMessages(streamConvId!, (prev) =>
              prev.map((m) => (m.id === curId ? { ...m, content: m.content + text } : m)),
            );
          },
          onMeta: (meta) => {
            if (!isRunCurrent()) return;
            if (meta.phase === "cancelled") {
              sealStreamingMessagesForConv(streamConvId!, true);
              if (isViewingStream()) {
                setSending(false);
                setRunLabel("正在思考…");
              }
            }
            if (meta.phase === "message_saved" && typeof meta.message_id === "string") {
              stampSavedMessage(streamConvId!, meta.message_id);
            }
            if (meta.phase === "task_queued") {
              noteTask(streamConvId!, true, "正在做，做好会发在这里");
            }
          },
          onStatus: (data) => {
            if (!isRunCurrent()) return;
            const phase = String(data.phase || "");
            const tool = typeof data.tool === "string" ? data.tool : "";
            if (typeof data.label === "string" && data.label.trim()) {
              if (phase === "tool" && tool) {
                setRunLabelForStream(`${data.label} · ${tool}`);
              } else {
                setRunLabelForStream(data.label);
              }
              return;
            }
            if (phase === "tool") {
              setRunLabelForStream(tool ? `正在运行命令 · ${tool}` : "正在运行命令");
            } else if (phase === "thinking" || phase === "tool_done") {
              setRunLabelForStream("正在思考…");
            }
          },
          onError: (msg) => {
            if (!isRunCurrent()) return;
            sendHadError = true;
            setStatus(`错误：${msg}`);
            setRunLabelForStream("正在思考…");
            const curId = runsRef.current.get(streamConvId!)?.assistantId;
            patchConvMessages(streamConvId!, (prev) =>
              prev.map((m) =>
                m.id === curId
                  ? { ...m, streaming: false, content: m.content || `（失败）${msg}` }
                  : m,
              ),
            );
          },
          onDone: () => {
            if (!isRunCurrent()) return;
            const curId = runsRef.current.get(streamConvId!)?.assistantId;
            patchConvMessages(streamConvId!, (prev) =>
              prev.map((m) => (m.id === curId ? { ...m, streaming: false } : m)),
            );
            if (!taskConvIdsRef.current.has(streamConvId!)) {
              setRunLabelForStream("正在思考…");
            }
            void refreshConversations();
          },
        },
        ac.signal,
        uploaded.length ? uploaded : undefined,
        mentionAgentIds,
        { ...clientEnv, machine_id: getStoredMachineId() || undefined },
      );
      if (!sendHadError && sendSelection) {
        saveLastActiveSelection(sendSelection);
      }
    } catch (err) {
      const aborted =
        (err instanceof DOMException && err.name === "AbortError") ||
        (err instanceof Error && err.name === "AbortError");
      if (aborted) {
        // stopCurrentRun already sealed bubbles; ignore stale aborts from interrupt-and-send.
        return;
      }
      if (streamConvId && !((runsRef.current.get(streamConvId)?.generation ?? 0) === gen)) return;
      const msg = err instanceof Error ? err.message : String(err);
      setStatus(`发送失败：${msg}`);
      if (streamConvId) {
        const curId = runsRef.current.get(streamConvId)?.assistantId ?? assistantId;
        patchConvMessages(streamConvId, (prev) =>
          prev.map((m) =>
            m.id === curId
              ? { ...m, streaming: false, content: m.content || `（失败）${msg}` }
              : m,
          ),
        );
        if (conversationRef.current?.id === streamConvId) {
          setRunLabel("正在思考…");
        }
      } else {
        setRunLabel("正在思考…");
      }
    } finally {
      if (streamConvId) {
        const run = runsRef.current.get(streamConvId);
        if (run && run.generation === gen) {
          runsRef.current.delete(streamConvId);
          bumpRunBusy();
          if (conversationRef.current?.id === streamConvId) {
            setSending(false);
          }
        }
      } else if (conversationRef.current?.id === viewedId) {
        setSending(false);
      }
    }
  };

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault();
    await sendUserText(input, [...pendingFiles]);
  };


  const refreshMCP = useCallback(async () => {
    const list = await listMCPServers();
    setMcpServers(list);
  }, []);

  const refreshRoutines = useCallback(async () => {
    const list = await listRoutines();
    setRoutines(list);
  }, []);

  const saveMCP = async (e: FormEvent) => {
    e.preventDefault();
    setMcpBusy(true);
    setMcpMsg("");
    try {
      let args: string[] = [];
      try {
        const parsed = JSON.parse(mcpArgsText || "[]");
        if (!Array.isArray(parsed)) throw new Error("args 须为 JSON 数组");
        args = parsed.map(String);
      } catch (err) {
        throw new Error(err instanceof Error ? err.message : "args JSON 无效");
      }
      const body: MCPServerInput = {
        name: mcpForm.name,
        transport: mcpForm.transport,
        command: mcpForm.command || "",
        args,
        url: mcpForm.url || "",
        enabled: mcpForm.enabled !== false,
      };
      await createMCPServer(body);
      setMcpMsg("已创建");
      setMcpForm({
        name: "",
        transport: "stdio",
        command: "",
        args: [],
        url: "",
        enabled: true,
      });
      setMcpArgsText("[]");
      await refreshMCP();
    } catch (err) {
      setMcpMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setMcpBusy(false);
    }
  };

  const toggleMCP = async (s: MCPServer, enabled: boolean) => {
    setMcpBusy(true);
    setMcpMsg("");
    try {
      await updateMCPServer(s.id, { enabled });
      await refreshMCP();
    } catch (err) {
      setMcpMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setMcpBusy(false);
    }
  };

  const removeMCP = async (id: string) => {
    const ok = await confirm({
      title: "删除该 MCP server？",
      confirmLabel: "删除",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setMcpBusy(true);
    setMcpMsg("");
    try {
      await deleteMCPServer(id);
      await refreshMCP();
    } catch (err) {
      setMcpMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setMcpBusy(false);
    }
  };

  const runTestMCP = async (id: string) => {
    setMcpBusy(true);
    setMcpTestResult("");
    setMcpMsg("");
    try {
      const r = await testMCPServer(id);
      if (r.ok) {
        const names = r.tool_names || (r.tools || []).map((t) => t.name);
        setMcpTestResult(`连接成功 · 工具: ${names.join(", ") || "(无)"}`);
      } else {
        setMcpTestResult(`失败: ${r.error || "unknown"}`);
      }
    } catch (err) {
      setMcpTestResult(err instanceof Error ? err.message : String(err));
    } finally {
      setMcpBusy(false);
    }
  };

  const runCallMCP = async (e: FormEvent) => {
    e.preventDefault();
    setMcpBusy(true);
    setMcpCallResult("");
    try {
      let args: Record<string, unknown> = {};
      try {
        args = JSON.parse(mcpCallArgs || "{}") as Record<string, unknown>;
      } catch {
        throw new Error("arguments 须为 JSON 对象");
      }
      if (!mcpCallServerId) throw new Error("请选择 server");
      const r = await mcpCallTool({
        server_id: mcpCallServerId,
        tool: mcpCallToolName,
        arguments: args,
      });
      setMcpCallResult(
        r.error
          ? `错误: ${r.error}`
          : r.text || JSON.stringify(r, null, 2),
      );
    } catch (err) {
      setMcpCallResult(err instanceof Error ? err.message : String(err));
    } finally {
      setMcpBusy(false);
    }
  };


  const saveRoutine = async (e: FormEvent) => {
    e.preventDefault();
    setRoutinesBusy(true);
    setRoutinesMsg("");
    try {
      await createRoutine({
        name: routineForm.name.trim(),
        prompt: routineForm.prompt.trim(),
        schedule_cron: routineForm.schedule_cron.trim() || "0 9 * * *",
        enabled: routineForm.enabled !== false,
      });
      setRoutineForm({ name: "", prompt: "", schedule_cron: "0 9 * * *", enabled: true });
      await refreshRoutines();
      setRoutinesMsg("已创建");
    } catch (err) {
      setRoutinesMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setRoutinesBusy(false);
    }
  };

  const toggleRoutine = async (r: Routine, enabled: boolean) => {
    setRoutinesBusy(true);
    setRoutinesMsg("");
    try {
      await updateRoutine(r.id, { enabled });
      await refreshRoutines();
    } catch (err) {
      setRoutinesMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setRoutinesBusy(false);
    }
  };

  const removeRoutine = async (id: string) => {
    const ok = await confirm({
      title: "删除该例行任务？",
      confirmLabel: "删除",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setRoutinesBusy(true);
    setRoutinesMsg("");
    try {
      await deleteRoutine(id);
      await refreshRoutines();
    } catch (err) {
      setRoutinesMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setRoutinesBusy(false);
    }
  };

  const runRoutineNow = async (id: string) => {
    setRoutinesBusy(true);
    setRoutinesMsg("运行中…");
    try {
      const res = await runRoutine(id);
      await refreshRoutines();
      setRoutinesMsg(`运行完成：${res.run.status}`);
    } catch (err) {
      setRoutinesMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setRoutinesBusy(false);
    }
  };

  const refreshSandbox = useCallback(async () => {
    const s = await getSandbox();
    setSandbox(s);
  }, []);

  const refreshSecrets = useCallback(async () => {
    try {
      const [secs, reqs] = await Promise.all([listBotSecrets(), listBotSecretRequests()]);
      setBotSecrets(secs);
      setSecretRequests(reqs);
      if (reqs.length && !secretPrompt) setSecretPrompt(reqs[0]);
    } catch {
      /* soft-fail */
    }
  }, [secretPrompt]);
  useEffect(() => {
    if (!getToken()) return;
    void refreshSecrets();
    const id = window.setInterval(() => void refreshSecrets(), 8000);
    return () => window.clearInterval(id);
  }, [refreshSecrets]);


  const doCheckpointSandbox = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const res = await checkpointSandbox();
      setSandbox(res.sandbox);
      setSandboxMsg(`已快照：${res.checkpoint_path}`);
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doEnsureSandbox = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const s = await ensureSandbox();
      setSandbox(s);
      setSandboxMsg(`已确保运行：${s.container_id || "(no id)"}`);
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doOpenDesktop = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const s = await ensureSandbox({ desktop: true });
      setSandbox(s);
      if (!s.desktop_port) {
        setSandboxMsg(
          s.last_error ||
            "桌面预览未就绪：请先构建桌面镜像（make sandbox-image-desktop）",
        );
        return;
      }
      const url = sandboxDesktopURL({ desktop_token: s.desktop_token });
      window.open(url, "openbot-sandbox-desktop", "noopener,noreferrer");
      setSandboxMsg(`桌面已打开（本地端口 ${s.desktop_port}，经 API JWT 反代）`);
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doStopSandbox = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const s = await stopSandbox();
      setSandbox(s);
      setSandboxMsg("已停止");
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doResetSandbox = async () => {
    const ok = await confirm({
      title: "重置将清空运行环境中的文件并重建，确认？",
      confirmLabel: "重置",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const res = await resetSandbox();
      setSandbox(res.sandbox);
      setSandboxMsg(res.warning || "已重置");
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doExecSandbox = async (e: FormEvent) => {
    e.preventDefault();
    setSandboxBusy(true);
    setSandboxMsg("");
    setSandboxExecOut("");
    try {
      const res = await execSandbox({ cmd: sandboxCmd });
      setSandboxExecOut(
        `exit=${res.exit_code}\n--- stdout ---\n${res.stdout}\n--- stderr ---\n${res.stderr}`,
      );
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doLsSandbox = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const res = await listSandbox(sandboxPath || "/workspace");
      const lines = (res.entries || []).map(
        (e) => `${e.is_dir ? "d" : "f"}\t${e.name}\t${e.size}`,
      );
      setSandboxLsOut(lines.join("\n") || "(empty)");
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doWriteSandboxFile = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      await writeSandboxFile(sandboxFilePath, sandboxFileContent);
      setSandboxMsg(`已写入 ${sandboxFilePath}`);
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const doReadSandboxFile = async () => {
    setSandboxBusy(true);
    setSandboxMsg("");
    try {
      const res = await readSandboxFile(sandboxFilePath);
      setSandboxFileContent(res.content);
      setSandboxMsg(`已读取 ${res.path}`);
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSandboxBusy(false);
    }
  };

  const startChatWithAgent = async (agent: Agent) => {
    setShowNewChat(false);
    onSelectAgent(agent.id);
  };

  const createBotFromPopover = async (input: CreateBotInput) => {
    const created = await createAgent({
      name: input.name,
      description: input.description,
      system_prompt: input.system_prompt,
    });
    await refreshAgents();
    setShowNewChat(false);
    // Navigate away without cancelling other bots' in-flight runs.
    snapshotViewedMessages();
    selectGenRef.current += 1;
    setAgentId(created.id);
    setPendingFiles([]);
    setSending(false);
    setRunLabel("正在思考…");
    setStatus("");
    toast.success(`已创建 Bot「${created.name}」`);
    try {
      const conv = await openPrimaryConversation(created.id);
      stickToBottomRef.current = true;
      conversationRef.current = conv;
      setConversation(conv);
      messagesByConvRef.current.set(conv.id, []);
      messagesLiveRef.current = [];
      setMessages([]);
      setOnboardingDismissedState(false);
      saveLastActiveSelection({ kind: "agent", id: created.id });
      await refreshConversations();
      await refreshAgents();
    } catch (err) {
      conversationRef.current = null;
      setConversation(null);
      setMessages([]);
      setOnboardingDismissedState(false);
      setStatus(err instanceof Error ? err.message : String(err));
    }
  };

  const createGroupFromPopover = async (name: string, memberIds: string[]) => {
    const ch = await createChannel(name, memberIds);
    await refreshChannels();
    setShowNewChat(false);
    toast.success(`已创建群聊「${ch.name}」（${memberIds.length} 名成员）`);
    // Open linked conversation immediately (no longer flash-only).
    await openChannelChat(ch.id);
    await refreshConversations();
  };

  useEffect(() => {
    if (!authed || !user?.id) {
      didRestoreLastActive.current = null;
      return;
    }
    if (didRestoreLastActive.current === user.id) return;

    let cancelled = false;
    const restoreLastActive = async () => {
      try {
        const [agentList, channelList] = await Promise.all([refreshAgents(), refreshChannels()]);
        if (cancelled || didRestoreLastActive.current === user.id || conversationRef.current) return;

        // Mark the auth session as handled before opening the restored conversation.
        didRestoreLastActive.current = user.id;
        const saved = readLastActiveSelection(user.id);
        if (saved?.kind === "agent" && agentList.some((agent) => agent.id === saved.id)) {
          onSelectAgent(saved.id);
          return;
        }
        if (saved?.kind === "channel" && channelList.some((channel) => channel.id === saved.id)) {
          void openChannelChat(saved.id);
          return;
        }

        const datedAgents = agentList
          .filter((agent) => agent.conversation_updated_at)
          .sort(
            (a, b) =>
              (Date.parse(b.conversation_updated_at || "") || 0) -
              (Date.parse(a.conversation_updated_at || "") || 0),
          );
        const fallback = datedAgents[0] ?? agentList[0];
        if (fallback) onSelectAgent(fallback.id);
      } catch {
        if (!cancelled) setStatus("无法拉取助手或群聊列表（请确认已登录且 API 可用）");
      }
    };

    void restoreLastActive();
    void refreshLLMs().catch(() => setStatus("无法拉取 LLM 连接列表"));
    return () => {
      cancelled = true;
    };
  }, [authed, user?.id, refreshAgents, refreshChannels, refreshLLMs, onSelectAgent, openChannelChat]);

  const openSettings = async (tab: SettingsTab = "llm") => {
    setShowSettings(true);
    setSettingsTab(tab);
    setLLMMsg("");
    setSkillsMsg("");
    setMcpMsg("");
    setMcpTestResult("");
    try {
      await refreshLLMs();
    } catch (err) {
      setLLMMsg(err instanceof Error ? err.message : String(err));
    }
    try {
      await refreshAgents();
    } catch {
      /* sidebar still shows fallback agents */
    }
    try {
      await refreshSkills();
    } catch (err) {
      setSkillsMsg(err instanceof Error ? err.message : String(err));
    }
    try {
      await refreshCompact();
    } catch {
      /* optional */
    }
    try {
      await refreshMCP();
    } catch (err) {
      setMcpMsg(err instanceof Error ? err.message : String(err));
    }
    setRoutinesMsg("");
    try {
      await refreshRoutines();
    } catch (err) {
      setRoutinesMsg(err instanceof Error ? err.message : String(err));
    }
    setSandboxMsg("");
    try {
      await refreshSandbox();
    } catch (err) {
      setSandboxMsg(err instanceof Error ? err.message : String(err));
    }
  };

  const startEdit = (c: LLMConnection) => {
    setEditingId(c.id);
    setLLMForm({
      name: c.name,
      base_url: c.base_url,
      api_key: "",
      model: c.model,
      enable_tools: c.enable_tools,
      is_default: c.is_default,
      context_window: c.context_window ?? null,
    });
    setLLMMsg(c.api_key_set ? `已保存密钥 ${c.api_key_hint || ""}（留空则不修改）` : "");
  };

  const resetForm = () => {
    setEditingId(null);
    setLLMForm({ ...emptyLLMForm, is_default: llms.length === 0 });
    setLLMMsg("");
  };

  const saveLLM = async (e: FormEvent) => {
    e.preventDefault();
    setLLMBusy(true);
    setLLMMsg("");
    try {
      if (editingId) {
        const patch: Partial<LLMInput> = {
          name: llmForm.name,
          base_url: llmForm.base_url,
          model: llmForm.model,
          enable_tools: llmForm.enable_tools,
          is_default: llmForm.is_default,
          context_window:
            llmForm.context_window && llmForm.context_window > 0
              ? llmForm.context_window
              : null,
        };
        if (llmForm.api_key && llmForm.api_key.trim()) {
          patch.api_key = llmForm.api_key.trim();
        }
        await updateLLMConnection(editingId, patch);
        setLLMMsg("已更新");
      } else {
        await createLLMConnection({
          ...llmForm,
          context_window:
            llmForm.context_window && llmForm.context_window > 0
              ? llmForm.context_window
              : null,
        });
        setLLMMsg("已创建");
      }
      await refreshLLMs();
      resetForm();
    } catch (err) {
      setLLMMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setLLMBusy(false);
    }
  };

  const removeAgent = async (a: Agent, e?: MouseEvent) => {
    e?.stopPropagation();
    const ok = await confirm({
      title: `确定删除助手「${a.name}」？`,
      confirmLabel: "删除",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setAgentBusy(true);
    try {
      await deleteAgent(a.id);
      const list = await listAgents();
      setAgents(list);
      await refreshConversations().catch(() => {});
      // Cancel any in-flight runs owned by this agent (viewed or background).
      const agentKey = `agent:${a.id}`;
      for (const [convId, run] of [...runsRef.current.entries()]) {
        if (run.selectionKey === agentKey) {
          await stopCurrentRun({ markStopped: true, conversationId: convId });
        }
      }
      if (agentId === a.id) {
        setAgentId(list[0]?.id ?? "");
        conversationRef.current = null;
        setConversation(null);
        setMessages([]);
        setSending(false);
        setStatus("");
      } else if (conversation?.agent_id === a.id && !conversation?.channel_id) {
        conversationRef.current = null;
        setConversation(null);
        setMessages([]);
        setSending(false);
        setStatus("");
      }
      toast.success(`已删除助手「${a.name}」`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setAgentBusy(false);
    }
  };


  const removeChannel = async (ch: Channel, e?: MouseEvent) => {
    e?.stopPropagation();
    const ok = await confirm({
      title: `确定删除群聊「${ch.name}」？`,
      confirmLabel: "删除",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setChannelBusy(true);
    try {
      await deleteChannel(ch.id);
      await refreshChannels();
      const channelKey = `channel:${ch.id}`;
      for (const [convId, run] of [...runsRef.current.entries()]) {
        if (run.selectionKey === channelKey || (ch.conversation_id && convId === ch.conversation_id)) {
          await stopCurrentRun({ markStopped: true, conversationId: convId });
        }
      }
      if (conversation?.channel_id === ch.id || conversation?.id === ch.conversation_id) {
        conversationRef.current = null;
        setConversation(null);
        setMessages([]);
        setSending(false);
        setStatus("");
      }
      toast.success(`已删除群聊「${ch.name}」`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err));
    } finally {
      setChannelBusy(false);
    }
  };


  const onUploadSkill = async (e: FormEvent) => {
    e.preventDefault();
    setSkillsBusy(true);
    setSkillsMsg("");
    try {
      await uploadSkill({
        name: skillName.trim(),
        description: skillDesc.trim(),
        body_markdown: skillBody,
      });
      setSkillName("");
      setSkillDesc("");
      setSkillBody("");
      await refreshSkills();
      setSkillsMsg("技能已上传并默认启用");
    } catch (err) {
      setSkillsMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSkillsBusy(false);
    }
  };

  const onDeleteSkill = async (name: string) => {
    const ok = await confirm({
      title: `删除自定义技能「${name}」？`,
      confirmLabel: "删除",
      cancelLabel: "取消",
      danger: true,
    });
    if (!ok) return;
    setSkillsBusy(true);
    setSkillsMsg("");
    try {
      await deleteSkill(name);
      await refreshSkills();
      setSkillsMsg("已删除");
    } catch (err) {
      setSkillsMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSkillsBusy(false);
    }
  };

  const toggleSkill = async (name: string, enabled: boolean) => {
    setSkillsBusy(true);
    setSkillsMsg("");
    try {
      await setSkillEnabled(name, enabled);
      await refreshSkills();
    } catch (err) {
      setSkillsMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setSkillsBusy(false);
    }
  };

  if (!authed) {
    return (
      <div className="auth-page">
        <form className="auth-card" onSubmit={onAuth}>
          <div className="brand auth-brand">
            <div className="logo">◈</div>
            <div>
              <div className="brand-title">open-bot</div>
              <div className="brand-sub">登录后使用自定义 LLM</div>
            </div>
          </div>
          <div className="auth-tabs">
            <button
              type="button"
              className={authMode === "login" ? "active" : ""}
              onClick={() => setAuthMode("login")}
            >
              登录
            </button>
            <button
              type="button"
              className={authMode === "register" ? "active" : ""}
              onClick={() => setAuthMode("register")}
            >
              注册
            </button>
          </div>
          <label>
            用户名
            <input
              value={authUser}
              onChange={(e) => setAuthUser(e.target.value)}
              autoComplete="username"
              required
              minLength={2}
            />
          </label>
          <label>
            密码
            <input
              type="password"
              value={authPass}
              onChange={(e) => setAuthPass(e.target.value)}
              autoComplete={authMode === "login" ? "current-password" : "new-password"}
              required
              minLength={4}
            />
          </label>
          {authError ? <div className="auth-error">{authError}</div> : null}
          <button type="submit" className="primary" disabled={authBusy}>
            {authBusy ? "请稍候…" : authMode === "login" ? "登录" : "注册"}
          </button>
          {oidcEnabled && authMode === "login" ? (
            <>
              <div className="auth-divider muted small">或</div>
              <button type="button" className="settings-btn" disabled={authBusy} onClick={() => void onCasdoorLogin()}>
                用 Casdoor 登录
              </button>
            </>
          ) : null}
          <p className="muted small">API：{API_BASE}</p>
        </form>
      </div>
    );
  }

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-top">
          <label className="sidebar-search">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="7" />
              <path d="M20 20l-3.5-3.5" />
            </svg>
            <input
              value={convSearch}
              onChange={(e) => setConvSearch(e.target.value)}
              placeholder="搜索助手 / 群聊"
              aria-label="搜索助手或群聊"
            />
          </label>
          <button
            ref={newChatBtnRef}
            type="button"
            className={`icon-btn${showNewChat ? " active-plus" : ""}`}
            title="新建聊天"
            aria-haspopup="dialog"
            aria-expanded={showNewChat}
            onClick={() => setShowNewChat((v) => !v)}
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 5v14M5 12h14" />
            </svg>
          </button>
        </div>

        <div className="section-label">助手</div>
        <nav className="agent-list">
          {agents.length === 0 ? (
            <div className="muted small" style={{ padding: "8px 10px", lineHeight: 1.5 }}>
              暂无助手。点击右上角「+」创建第一个助手。
            </div>
          ) : filteredAgents.length === 0 ? (
            <div className="muted small" style={{ padding: "4px 10px" }}>
              无匹配助手
            </div>
          ) : (
            filteredAgents.map((a) => {
              const active =
                !conversation?.channel_id &&
                (conversation?.agent_id === a.id || (!conversation && a.id === agentId));
              const snippet = (a.last_message || a.description || "").trim();
              return (
                <div
                  key={a.id}
                  className={`agent-item conv-item ${active ? "active" : ""}`}
                  onClick={() => onSelectAgent(a.id)}
                  role="button"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      onSelectAgent(a.id);
                    }
                  }}
                >
                  <AgentAvatar id={a.id} name={a.name} size={32} />
                  <div className="agent-item-body">
                    <div className="agent-name">
                      {a.name}
                      {busySelectionKeys.has(`agent:${a.id}`) ? (
                        <span className="run-busy-dot" title="回复中" aria-label="回复中" />
                      ) : null}
                    </div>
                    <div className="agent-desc">{snippet || "尚开始对话"}</div>
                  </div>
                  <button
                    type="button"
                    className="conv-del"
                    title="删除助手"
                    disabled={agentBusy}
                    onClick={(e) => void removeAgent(a, e)}
                  >
                    ×
                  </button>
                </div>
              );
            })
          )}
        </nav>

        <div className="section-label">群聊</div>
        <div className="conv-list channel-list">
          {filteredChannels.length === 0 ? (
            <div className="muted small" style={{ padding: "4px 10px" }}>
              {channels.length === 0 ? "暂无群聊，可从「+」创建" : "无匹配群聊"}
            </div>
          ) : (
            filteredChannels.map((ch) => {
              const memberNames = (ch.members || [])
                .map((id) => agentNameById.get(id) || id)
                .join("、");
              return (
                <div
                  key={ch.id}
                  className={`agent-item conv-item channel-item ${conversation?.channel_id === ch.id || conversation?.id === ch.conversation_id ? "active" : ""}`}
                  title={memberNames}
                  role="button"
                  tabIndex={0}
                  onClick={() => void openChannelChat(ch.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      (e.currentTarget as HTMLElement).click();
                    }
                  }}
                >
                  <div className="conv-item-main">
                    <div className="agent-name">
                      {ch.name}
                      {busySelectionKeys.has(`channel:${ch.id}`) ? (
                        <span className="run-busy-dot" title="回复中" aria-label="回复中" />
                      ) : null}
                    </div>
                    <div className="agent-desc">
                      {memberNames || "无成员"} · 点击打开群聊
                    </div>
                  </div>
                  <button
                    type="button"
                    className="conv-del"
                    title="删除群聊"
                    disabled={channelBusy}
                    onClick={(e) => void removeChannel(ch, e)}
                  >
                    ×
                  </button>
                </div>
              );
            })
          )}
        </div>

        <div className="sidebar-foot">
          <AccountMenu
            username={user?.username || "账户"}
            llm={defaultLLM}
            onOpenSettings={() => void openSettings("general")}
            onChangeModel={() => void openSettings("llm")}
            onLogout={onLogout}
          />
        </div>
      </aside>

      <NewChatPopover
        open={showNewChat}
        agents={agents}
        anchorRef={newChatBtnRef}
        onClose={() => setShowNewChat(false)}
        onSelectAgent={(a) => void startChatWithAgent(a)}
        onCreateBot={(input) => createBotFromPopover(input)}
        onCreateGroup={(name, ids) => createGroupFromPopover(name, ids)}
      />

      <main className="main">
        {hostActivity ? <div className="status host-activity">{hostActivity}</div> : null}
        <header className="topbar">
          <div className="agent-pill">
            {activeChannel ? (
              <>
                <span className="agent-pill-name">{activeChannel.name}</span>
                <span className="muted small" style={{ marginLeft: 8 }}>
                  {(activeChannel.members || [])
                    .map((id) => agentNameById.get(id) || id)
                    .join("、")}
                </span>
              </>
            ) : (
              <>
                <AgentAvatar
                  id={activeAgent?.id}
                  name={activeAgent?.name ?? "助手"}
                  size={28}
                />
                <span className="agent-pill-name">{activeAgent?.name ?? "助手"}</span>
              </>
            )}
          </div>
        </header>

        <div className="messages" ref={messagesRef} onScroll={onMessagesScroll}>
          {showOnboarding ? (
            <>
              {ONBOARDING_WELCOME.map((text, i) => (
                <ChatMessage
                  key={`onboard-welcome-${i}`}
                  message={{ id: `onboard-welcome-${i}`, role: "assistant", content: text }}
                />
              ))}
              <BotOnboardingCard
                disabled={sending}
                onSelectOption={(opt: OnboardingOption) => {
                  void sendUserText(opt.prompt);
                }}
                onCustomSubmit={(customText: string) => {
                  void sendUserText(customText);
                }}
                onDismiss={() => {
                  setOnboardingDismissed(onboardingKey);
                  setOnboardingDismissedState(true);
                }}
              />
            </>
          ) : messages.length === 0 ? (
            <div className="empty">
              <h2>有什么可以帮你的？</h2>
              <p>{agents.length === 0 ? "暂无助手，点击左上角「+」创建一个。" : "从左侧选择一位助手进入连续对话；群聊里可用 @ 点名助手。回复经 Go API 流式代理到 Python runtime。"}</p>
            </div>
          ) : null}
          {messages.map((m) => (
            <ChatMessage
              key={m.id}
              message={m}
              agentId={agentId}
              onHostDecide={m.role === "host_confirm" ? (ok) => settleHostConfirm(m, ok) : undefined}
            />
          ))}
          {sending || taskBusy ? (
            !messages.some((m) => m.role === "assistant" && m.streaming && m.content) ? (
              <RunStatus
                label={taskBusy && !sending ? runLabel || "正在做，做好会发在这里" : runLabel || "正在思考…"}
                color={avatarColor(activeAgent?.id || activeAgent?.name || "open-bot")}
              />
            ) : null
          ) : null}
        </div>

        <div className="composer-wrap">
          {showScrollBottom ? (
            <button
              type="button"
              className="scroll-bottom-btn"
              title="滚到底部"
              onClick={() => scrollToBottom(true)}
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
                <path d="M6 9l6 6 6-6" />
              </svg>
            </button>
          ) : null}
          <Composer
            value={input}
            onChange={setInput}
            onSubmit={onSubmit}
            onStop={() => stopCurrentRun({ markStopped: true })}
            sending={sending || taskBusy}
            agentName={activeChannel ? activeChannel.name : activeAgent?.name}
            files={pendingFiles}
            onFilesChange={setPendingFiles}
            mentionMembers={groupMentionMembers}
          />
        </div>
      </main>

      {showSettings && (
        <div className="modal-backdrop" onClick={() => setShowSettings(false)}>
          <div className="modal settings-dialog" onClick={(e) => e.stopPropagation()}>
            <nav className="settings-nav" aria-label="设置分类">
              {(
                [
                  ["general", "通用"],
                  ["machines", "电脑"],
                  ["sandbox", "运行环境"],
                  ["llm", "模型"],
                  ["skills", "Skills"],
                  ["mcp", "MCP"],
                  ["compact", "压缩"],
                  ["routines", "例行任务"],
                  ["secrets", "密钥"],
                ] as const
              ).map(([id, label]) => (
                <button
                  key={id}
                  type="button"
                  className={settingsTab === id ? "active" : ""}
                  onClick={() => {
                    setSettingsTab(id);
                    if (id === "mcp") {
                      void refreshMCP().catch((err) =>
                        setMcpMsg(err instanceof Error ? err.message : String(err)),
                      );
                    } else if (id === "compact") {
                      void refreshCompact();
                    } else if (id === "routines") {
                      void refreshRoutines().catch((err) =>
                        setRoutinesMsg(err instanceof Error ? err.message : String(err)),
                      );
                    } else if (id === "sandbox") {
                      void refreshSandbox().catch((err) =>
                        setSandboxMsg(err instanceof Error ? err.message : String(err)),
                      );
                    } else if (id === "machines") {
                      void refreshMachines().catch((err) =>
                        setMachinesMsg(err instanceof Error ? err.message : String(err)),
                      );
                    } else if (id === "secrets") {
                      void refreshSecrets();
                    }
                  }}
                >
                  <SettingsGlyph id={id} />
                  <span>{label}</span>
                </button>
              ))}
            </nav>
            <div className="settings-main">
              <div className="settings-main-head">
                <h2>{SETTINGS_TITLE[settingsTab]}</h2>
                <button
                  type="button"
                  className="settings-close"
                  aria-label="关闭"
                  onClick={() => setShowSettings(false)}
                >
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                    <path d="M6 6l12 12M18 6 6 18" />
                  </svg>
                </button>
              </div>
              <div className="settings-main-body">
            {settingsTab === "general" && (
              <div className="settings-page">
                <section className="settings-section">
                  <h3 className="settings-section-label">账户</h3>
                  <div className="settings-card settings-account">
                    <div className="settings-account-avatar" aria-hidden>
                      {accountInitials(user?.username || "")}
                    </div>
                    <div className="settings-account-main">
                      <div className="settings-account-name">{user?.username || "账户"}</div>
                      {user?.email ? (
                        <div className="settings-account-email">
                          <span>{user.email}</span>
                          <button
                            type="button"
                            className="settings-icon-btn"
                            title={emailCopied ? "已复制" : "复制邮箱"}
                            aria-label="复制邮箱"
                            onClick={() => {
                              void navigator.clipboard.writeText(user.email || "").then(() => {
                                setEmailCopied(true);
                                window.setTimeout(() => setEmailCopied(false), 1200);
                              });
                            }}
                          >
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
                              <rect x="8" y="8" width="12" height="12" rx="2" />
                              <path d="M4 16V6a2 2 0 0 1 2-2h10" />
                            </svg>
                          </button>
                        </div>
                      ) : null}
                    </div>
                    <button type="button" className="settings-pill" onClick={onLogout}>
                      退出登录
                    </button>
                  </div>
                </section>
                <section className="settings-section">
                  <h3 className="settings-section-label">模型</h3>
                  <div className="settings-card">
                    <div className="settings-row">
                      <span>默认模型</span>
                      <button type="button" className="settings-pill" onClick={() => setSettingsTab("llm")}>
                        {defaultLLM?.model || defaultLLM?.name || "未配置"}
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden>
                          <path d="M6 9l6 6 6-6" />
                        </svg>
                      </button>
                    </div>
                  </div>
                </section>
              </div>
            )}

            {settingsTab === "llm" && (
              <>
                <p className="muted small">
                  每个用户可配置多个 OpenAI 兼容连接；聊天默认使用「默认」连接。密钥不会完整回显。
                </p>
                <div className="llm-list">
                  {llms.length === 0 ? (
                    <div className="muted">暂无连接，请在下方新增。</div>
                  ) : (
                    llms.map((c) => (
                      <div key={c.id} className={`llm-item ${c.is_default ? "default" : ""}`}>
                        <div>
                          <div className="agent-name">
                            {c.name}
                            {c.is_default ? <span className="tag">默认</span> : null}
                          </div>
                          <div className="agent-desc">
                            {c.model || "(无 model)"} · {c.base_url || "(无 base_url)"} · key{" "}
                            {c.api_key_set ? c.api_key_hint || "已设置" : "未设置"}
                            {c.enable_tools ? " · tools" : ""}
                          </div>
                        </div>
                        <div className="llm-actions">
                          {!c.is_default ? (
                            <button
                              type="button"
                              onClick={() =>
                                void setDefaultLLMConnection(c.id)
                                  .then(refreshLLMs)
                                  .catch((err) => setLLMMsg(String(err)))
                              }
                            >
                              设默认
                            </button>
                          ) : null}
                          <button type="button" onClick={() => startEdit(c)}>
                            编辑
                          </button>
                          <button
                            type="button"
                            className="danger"
                            onClick={() =>
                              void deleteLLMConnection(c.id)
                                .then(refreshLLMs)
                                .then(resetForm)
                                .catch((err) => setLLMMsg(String(err)))
                            }
                          >
                            删除
                          </button>
                        </div>
                      </div>
                    ))
                  )}
                </div>

                <form className="llm-form" onSubmit={saveLLM}>
                  <h4>{editingId ? "编辑连接" : "新增连接"}</h4>
                  <label>
                    名称
                    <input
                      value={llmForm.name}
                      onChange={(e) => setLLMForm({ ...llmForm, name: e.target.value })}
                      required
                    />
                  </label>
                  <label>
                    Base URL
                    <input
                      value={llmForm.base_url}
                      onChange={(e) => setLLMForm({ ...llmForm, base_url: e.target.value })}
                      placeholder="https://api.openai.com/v1"
                    />
                  </label>
                  <label>
                    API Key{editingId ? "（留空不改）" : ""}
                    <input
                      type="password"
                      value={llmForm.api_key || ""}
                      onChange={(e) => setLLMForm({ ...llmForm, api_key: e.target.value })}
                      placeholder="sk-..."
                      autoComplete="off"
                    />
                  </label>
                  <label>
                    Model
                    <input
                      value={llmForm.model}
                      onChange={(e) => setLLMForm({ ...llmForm, model: e.target.value })}
                      placeholder="gpt-4o-mini"
                    />
                  </label>
                  <label>
                    上下文窗口 (tokens)
                    <input
                      type="number"
                      min={0}
                      step={1024}
                      value={llmForm.context_window ?? ""}
                      onChange={(e) => {
                        const raw = e.target.value.trim();
                        if (!raw) {
                          setLLMForm({ ...llmForm, context_window: null });
                          return;
                        }
                        const n = Number(raw);
                        setLLMForm({
                          ...llmForm,
                          context_window: Number.isFinite(n) && n > 0 ? Math.floor(n) : null,
                        });
                      }}
                      placeholder="留空=自动（按模型名推断）"
                    />
                  </label>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={Boolean(llmForm.enable_tools)}
                      onChange={(e) => setLLMForm({ ...llmForm, enable_tools: e.target.checked })}
                    />
                    启用 tools（上游需支持 tool calling）
                  </label>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={Boolean(llmForm.is_default)}
                      onChange={(e) => setLLMForm({ ...llmForm, is_default: e.target.checked })}
                    />
                    设为默认
                  </label>
                  {llmMsg ? <div className="status">{llmMsg}</div> : null}
                  <div className="llm-actions">
                    <button type="submit" className="primary" disabled={llmBusy}>
                      {llmBusy ? "保存中…" : "保存"}
                    </button>
                    {editingId ? (
                      <button type="button" onClick={resetForm}>
                        取消编辑
                      </button>
                    ) : null}
                  </div>
                </form>
              </>
            )}

            {settingsTab === "skills" && (
              <>
                <p className="muted small">
                  关闭后该技能不会注入 runtime 系统提示，也无法被 load_skill 加载。默认全部启用。
                  {"可上传自定义技能（Agent Skills：小写+数字+连字符），落盘到 skills/users/{user_id}/。"}
                </p>
                <form className="llm-form" onSubmit={(e) => void onUploadSkill(e)}>
                  <h4>上传自定义 Skill</h4>
                  <input
                    placeholder="name（如 my-helper）"
                    value={skillName}
                    onChange={(e) => setSkillName(e.target.value)}
                    required
                  />
                  <input
                    placeholder="description（何时使用）"
                    value={skillDesc}
                    onChange={(e) => setSkillDesc(e.target.value)}
                    required
                  />
                  <textarea
                    placeholder="正文 Markdown（可省略 frontmatter，会自动补全）"
                    value={skillBody}
                    onChange={(e) => setSkillBody(e.target.value)}
                    rows={6}
                  />
                  <button type="submit" disabled={skillsBusy}>
                    {skillsBusy ? "上传中…" : "上传并启用"}
                  </button>
                </form>
                <div className="llm-list">
                  {skills.length === 0 ? (
                    <div className="muted">暂无技能（检查仓库 skills/ 目录）</div>
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
                              disabled={skillsBusy}
                              onChange={(e) => void toggleSkill(s.name, e.target.checked)}
                            />
                            {s.enabled ? "已启用" : "已关闭"}
                          </label>
                          {s.custom ? (
                            <button
                              type="button"
                              className="ghost danger"
                              disabled={skillsBusy}
                              onClick={() => void onDeleteSkill(s.name)}
                            >
                              删除
                            </button>
                          ) : null}
                        </div>
                      </div>
                    ))
                  )}
                </div>
                {skillsMsg ? <div className="status">{skillsMsg}</div> : null}
              </>
            )}

            {settingsTab === "mcp" && (
              <>
                <p className="muted small">
                  配置 MCP Client servers（stdio / sse / http）。启用后，在 LLM 开启 tools 时会注入工具；本地
                  vLLM 无 function-calling 时可用下方手动「调用工具」。示例脚本：
                  <code>services/agent-runtime/examples/mcp_echo_server.py</code>
                </p>
                <div className="llm-list">
                  {mcpServers.length === 0 ? (
                    <div className="muted">暂无 MCP server，请在下方新增。</div>
                  ) : (
                    mcpServers.map((s) => (
                      <div key={s.id} className="llm-item">
                        <div>
                          <div className="agent-name">
                            {s.name}
                            {s.enabled ? <span className="tag">启用</span> : <span className="tag">停用</span>}
                          </div>
                          <div className="agent-desc">
                            {s.transport}
                            {s.transport === "stdio"
                              ? ` · ${s.command} ${(s.args || []).join(" ")}`
                              : ` · ${s.url}`}
                          </div>
                        </div>
                        <div className="llm-actions">
                          <button type="button" onClick={() => void runTestMCP(s.id)} disabled={mcpBusy}>
                            测试连接
                          </button>
                          <button
                            type="button"
                            onClick={() => void toggleMCP(s, !s.enabled)}
                            disabled={mcpBusy}
                          >
                            {s.enabled ? "停用" : "启用"}
                          </button>
                          <button
                            type="button"
                            className="danger"
                            onClick={() => void removeMCP(s.id)}
                            disabled={mcpBusy}
                          >
                            删除
                          </button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
                {mcpTestResult ? <div className="status">{mcpTestResult}</div> : null}
                {mcpMsg ? <div className="status">{mcpMsg}</div> : null}

                <form className="llm-form" onSubmit={saveMCP}>
                  <h4>新增 MCP server</h4>
                  <label>
                    名称
                    <input
                      value={mcpForm.name}
                      onChange={(e) => setMcpForm({ ...mcpForm, name: e.target.value })}
                      required
                    />
                  </label>
                  <label>
                    传输
                    <select
                      value={mcpForm.transport}
                      onChange={(e) =>
                        setMcpForm({
                          ...mcpForm,
                          transport: e.target.value as MCPServerInput["transport"],
                        })
                      }
                    >
                      <option value="stdio">stdio</option>
                      <option value="sse">sse</option>
                      <option value="http">http</option>
                    </select>
                  </label>
                  {mcpForm.transport === "stdio" ? (
                    <>
                      <label>
                        command（绝对路径或 python3/node/npx/uv…）
                        <input
                          value={mcpForm.command || ""}
                          onChange={(e) => setMcpForm({ ...mcpForm, command: e.target.value })}
                          placeholder="/path/to/.venv/bin/python"
                          required
                        />
                      </label>
                      <label>
                        args（JSON 数组）
                        <input
                          value={mcpArgsText}
                          onChange={(e) => setMcpArgsText(e.target.value)}
                          placeholder='["/path/to/mcp_echo_server.py"]'
                        />
                      </label>
                    </>
                  ) : (
                    <label>
                      URL
                      <input
                        value={mcpForm.url || ""}
                        onChange={(e) => setMcpForm({ ...mcpForm, url: e.target.value })}
                        placeholder="http://127.0.0.1:8000/sse"
                        required
                      />
                    </label>
                  )}
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={mcpForm.enabled !== false}
                      onChange={(e) => setMcpForm({ ...mcpForm, enabled: e.target.checked })}
                    />
                    启用
                  </label>
                  <div className="llm-actions">
                    <button type="submit" className="primary" disabled={mcpBusy}>
                      {mcpBusy ? "保存中…" : "新增"}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        // Fill echo example paths relative to common local layout.
                        setMcpForm({
                          name: "echo",
                          transport: "stdio",
                          command:
                            "/Users/tangxin/Workprojects/open-bot/services/agent-runtime/.venv/bin/python",
                          args: [
                            "/Users/tangxin/Workprojects/open-bot/services/agent-runtime/examples/mcp_echo_server.py",
                          ],
                          url: "",
                          enabled: true,
                        });
                        setMcpArgsText(
                          JSON.stringify([
                            "/Users/tangxin/Workprojects/open-bot/services/agent-runtime/examples/mcp_echo_server.py",
                          ]),
                        );
                        setMcpMsg("已填入本地 echo 示例路径");
                      }}
                    >
                      填入 echo 示例
                    </button>
                  </div>
                </form>

                <form className="llm-form" onSubmit={runCallMCP} style={{ marginTop: 16 }}>
                  <h4>手动调用工具</h4>
                  <label>
                    Server
                    <select
                      value={mcpCallServerId}
                      onChange={(e) => setMcpCallServerId(e.target.value)}
                    >
                      <option value="">选择…</option>
                      {mcpServers.map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Tool 名
                    <input
                      value={mcpCallToolName}
                      onChange={(e) => setMcpCallToolName(e.target.value)}
                      placeholder="echo"
                      required
                    />
                  </label>
                  <label>
                    Arguments（JSON）
                    <textarea
                      rows={3}
                      value={mcpCallArgs}
                      onChange={(e) => setMcpCallArgs(e.target.value)}
                    />
                  </label>
                  <div className="llm-actions">
                    <button type="submit" className="primary" disabled={mcpBusy}>
                      调用
                    </button>
                  </div>
                  {mcpCallResult ? (
                    <pre className="status" style={{ whiteSpace: "pre-wrap" }}>
                      {mcpCallResult}
                    </pre>
                  ) : null}
                </form>
              </>
            )}

            {settingsTab === "compact" && (
              <>
                <p className="muted small">
                  上下文压缩以估算 token 相对模型上下文窗口为主触发；消息数 / 字符数阈值作兜底。
                  每条 LLM 连接可在「LLM」页设置「上下文窗口 (tokens)」（留空则按模型名自动推断）。
                  摘要以 role=summary 落库，重启后优先复用。
                </p>
                {compactCfg ? (
                  <div className="llm-list">
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">触发模式</div>
                        <div className="agent-desc">
                          {compactCfg.token_mode === false ? "仅遗留阈值" : "token 预算（主）+ 遗留阈值（兜底）"}
                        </div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">上下文窗口（当前用于估算）</div>
                        <div className="agent-desc">
                          {defaultLLM?.context_window && defaultLLM.context_window > 0
                            ? `${defaultLLM.context_window}（默认 LLM「${defaultLLM.name}」显式设置）`
                            : `${compactCfg.context_window ?? compactCfg.default_context_window ?? "—"}（自动 / 环境默认）`}
                          {defaultLLM?.model ? ` · 模型 ${defaultLLM.model}` : ""}
                        </div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">估算 token 预算</div>
                        <div className="agent-desc">
                          {compactCfg.token_budget ?? "—"}
                          {" "}
                          （窗口 × {compactCfg.budget_ratio ?? 0.75} − 预留输出{" "}
                          {compactCfg.reserve_output_tokens ?? 2048}）
                        </div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_DEFAULT_CONTEXT_WINDOW</div>
                        <div className="agent-desc">{compactCfg.default_context_window ?? 32768}</div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_BUDGET_RATIO</div>
                        <div className="agent-desc">{compactCfg.budget_ratio ?? 0.75}</div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_RESERVE_OUTPUT_TOKENS</div>
                        <div className="agent-desc">{compactCfg.reserve_output_tokens ?? 2048}</div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_MAX_MESSAGES（兜底）</div>
                        <div className="agent-desc">{compactCfg.max_messages}</div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_MAX_CHARS（兜底）</div>
                        <div className="agent-desc">{compactCfg.max_chars}</div>
                      </div>
                    </div>
                    <div className="llm-item">
                      <div>
                        <div className="agent-name">COMPACT_KEEP_RECENT</div>
                        <div className="agent-desc">{compactCfg.keep_recent}</div>
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="muted">无法读取（请确认 runtime 已启动）</div>
                )}
                <button type="button" onClick={() => void refreshCompact()}>
                  刷新
                </button>
              </>
            )}

            {settingsTab === "routines" && (
              <>
                <p className="muted small">
                  例行任务：5 字段 cron（分 时 日 月 周）。调度器运行在 API 进程内，每分钟检查一次；使用指定助手（缺省为你的第一个助手）与默认 LLM。立即运行会写入 routine_runs。
                </p>
                <div className="llm-list">
                  {routines.length === 0 ? (
                    <div className="muted">暂无例行任务。</div>
                  ) : (
                    routines.map((r) => (
                      <div key={r.id} className="llm-item">
                        <div style={{ flex: 1 }}>
                          <div className="agent-name">
                            {r.name}
                            {r.enabled ? <span className="tag">启用</span> : <span className="tag">停用</span>}
                          </div>
                          <div className="agent-desc">
                            cron: {r.schedule_cron}
                            {r.last_run_at ? ` · 上次 ${r.last_run_at}` : ""}
                          </div>
                          <div className="agent-desc" style={{ marginTop: 4 }}>
                            prompt: {r.prompt.slice(0, 120)}
                            {r.prompt.length > 120 ? "…" : ""}
                          </div>
                          {r.last_run ? (
                            <pre
                              className="status"
                              style={{ whiteSpace: "pre-wrap", marginTop: 8, maxHeight: 160, overflow: "auto" }}
                            >
                              [{r.last_run.status}] {r.last_run.result_text}
                            </pre>
                          ) : null}
                        </div>
                        <div className="llm-actions">
                          <label className="check">
                            <input
                              type="checkbox"
                              checked={r.enabled}
                              onChange={(e) => void toggleRoutine(r, e.target.checked)}
                              disabled={routinesBusy}
                            />
                            启用
                          </label>
                          <button type="button" onClick={() => void runRoutineNow(r.id)} disabled={routinesBusy}>
                            立即运行
                          </button>
                          <button type="button" className="danger" onClick={() => void removeRoutine(r.id)}>
                            删除
                          </button>
                        </div>
                      </div>
                    ))
                  )}
                </div>

                <form className="llm-form" onSubmit={saveRoutine}>
                  <h4>新建例行任务</h4>
                  <label>
                    名称
                    <input
                      value={routineForm.name}
                      onChange={(e) => setRoutineForm({ ...routineForm, name: e.target.value })}
                      required
                    />
                  </label>
                  <label>
                    Prompt
                    <textarea
                      rows={3}
                      value={routineForm.prompt}
                      onChange={(e) => setRoutineForm({ ...routineForm, prompt: e.target.value })}
                      required
                    />
                  </label>
                  <label>
                    Cron（5 字段）
                    <input
                      value={routineForm.schedule_cron}
                      onChange={(e) => setRoutineForm({ ...routineForm, schedule_cron: e.target.value })}
                      placeholder="0 9 * * *"
                      required
                    />
                  </label>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={routineForm.enabled !== false}
                      onChange={(e) => setRoutineForm({ ...routineForm, enabled: e.target.checked })}
                    />
                    创建后启用
                  </label>
                  {routinesMsg ? <div className="status">{routinesMsg}</div> : null}
                  <div className="llm-actions">
                    <button type="submit" className="primary" disabled={routinesBusy}>
                      {routinesBusy ? "处理中…" : "创建"}
                    </button>
                    <button type="button" onClick={() => void refreshRoutines()}>
                      刷新
                    </button>
                  </div>
                </form>
              </>
            )}

            {settingsTab === "sandbox" && (
              <>
                <p className="muted small">
                  运行环境（高级）：助手生成脚本、临时文件与预览时使用的隔离环境。
                  一般用户只需在聊天里查看预览或下载结果；本机设备请看「我的电脑」。
                  「打开桌面」可打开图形预览（经 API 安全反代）。
                </p>
                <div className="llm-list">
                  <div className="llm-item">
                    <div>
                      <div className="agent-name">
                        状态：{sandbox?.status || "未知"}
                        {sandbox?.container_id ? (
                          <span className="tag">{sandbox.container_id.slice(0, 12)}</span>
                        ) : null}
                      </div>
                      <div className="agent-desc">
                        镜像：{sandbox?.image || "-"}
                        {sandbox?.workdir_host ? ` · 宿主机 ${sandbox.workdir_host}` : ""}
                      </div>
                      {sandbox?.last_error ? (
                        <div className="agent-desc" style={{ color: "#f88" }}>
                          错误：{sandbox.last_error}
                        </div>
                      ) : null}
                    </div>
                    <div className="llm-actions">
                      <button type="button" onClick={() => void doEnsureSandbox()} disabled={sandboxBusy}>
                        确保启动
                      </button>
                      <button type="button" onClick={() => void doOpenDesktop()} disabled={sandboxBusy}>
                        打开桌面
                      </button>
                      <button type="button" onClick={() => void doCheckpointSandbox()} disabled={sandboxBusy}>
                        快照
                      </button>
                      <button type="button" onClick={() => void doStopSandbox()} disabled={sandboxBusy}>
                        停止
                      </button>
                      <button
                        type="button"
                        className="danger"
                        onClick={() => void doResetSandbox()}
                        disabled={sandboxBusy}
                      >
                        重置
                      </button>
                      <button
                        type="button"
                        onClick={() =>
                          void refreshSandbox().catch((err) =>
                            setSandboxMsg(err instanceof Error ? err.message : String(err)),
                          )
                        }
                      >
                        刷新
                      </button>
                    </div>
                  </div>
                </div>

                <form className="llm-form" onSubmit={doExecSandbox}>
                  <h4>执行命令</h4>
                  <label>
                    命令
                    <input
                      value={sandboxCmd}
                      onChange={(e) => setSandboxCmd(e.target.value)}
                      placeholder="echo hi"
                      required
                    />
                  </label>
                  <div className="llm-actions">
                    <button type="submit" className="primary" disabled={sandboxBusy}>
                      运行
                    </button>
                  </div>
                  {sandboxExecOut ? (
                    <pre
                      className="status"
                      style={{ whiteSpace: "pre-wrap", maxHeight: 220, overflow: "auto" }}
                    >
                      {sandboxExecOut}
                    </pre>
                  ) : null}
                </form>

                <div className="llm-form">
                  <h4>列目录 / 读写文件</h4>
                  <label>
                    路径（ls）
                    <input
                      value={sandboxPath}
                      onChange={(e) => setSandboxPath(e.target.value)}
                      placeholder="/workspace"
                    />
                  </label>
                  <div className="llm-actions">
                    <button type="button" onClick={() => void doLsSandbox()} disabled={sandboxBusy}>
                      列出
                    </button>
                  </div>
                  {sandboxLsOut ? (
                    <pre
                      className="status"
                      style={{ whiteSpace: "pre-wrap", maxHeight: 160, overflow: "auto" }}
                    >
                      {sandboxLsOut}
                    </pre>
                  ) : null}
                  <label>
                    文件路径
                    <input
                      value={sandboxFilePath}
                      onChange={(e) => setSandboxFilePath(e.target.value)}
                      placeholder="hello.txt"
                    />
                  </label>
                  <label>
                    内容
                    <textarea
                      rows={3}
                      value={sandboxFileContent}
                      onChange={(e) => setSandboxFileContent(e.target.value)}
                    />
                  </label>
                  <div className="llm-actions">
                    <button type="button" onClick={() => void doWriteSandboxFile()} disabled={sandboxBusy}>
                      写入
                    </button>
                    <button type="button" onClick={() => void doReadSandboxFile()} disabled={sandboxBusy}>
                      读取
                    </button>
                  </div>
                </div>
                {sandboxMsg ? <div className="status">{sandboxMsg}</div> : null}
              </>
            )}
            {settingsTab === "machines" && (
              <>
                <p className="muted small">
                  已注册的电脑。桌面端登录后会连上本机文件通道，只有这时才显示为可操作。
                  网页不会登记为电脑。可读写 Downloads、Desktop、Documents。
                  覆盖、删除和移动会在那台电脑上请你确认。
                  当前客户端：{clientEnv.platform} / {clientEnv.app}
                  {shouldRegisterAsHost(clientEnv) ? "（会自动注册）" : "（浏览器，不自动注册）"}。
                </p>
                {clientEnv.app === "tauri" ? (
                  <label className="muted small">
                    <input
                      type="checkbox"
                      checked={hostWritesOn}
                      onChange={(e) => {
                        setHostWritesEnabled(e.target.checked);
                        setHostWritesOn(e.target.checked);
                      }}
                    />{" "}
                    允许写入这台电脑
                  </label>
                ) : null}
                <div className="llm-actions" style={{ marginBottom: 12 }}>
                  <button
                    type="button"
                    onClick={() =>
                      void refreshMachines()
                        .then(() => setMachinesMsg("已刷新"))
                        .catch((err) =>
                          setMachinesMsg(err instanceof Error ? err.message : String(err)),
                        )
                    }
                    disabled={machinesBusy}
                  >
                    刷新
                  </button>
                  {shouldRegisterAsHost(clientEnv) ? (
                    <button
                      type="button"
                      className="primary"
                      disabled={machinesBusy}
                      onClick={() => {
                        setMachinesBusy(true);
                        void registerMachine({
                          machine_key: getOrCreateMachineKey(),
                          label: defaultMachineLabel(clientEnv),
                          platform: clientEnv.platform,
                          os: clientEnv.os,
                          arch: clientEnv.arch,
                          app: clientEnv.app,
                          app_version: clientEnv.app_version,
                        })
                          .then((m) => {
                            setStoredMachineId(m.id);
                            setHostMachineId(m.id);
                            setMachinesMsg(`已注册：${m.label}`);
                            return refreshMachines();
                          })
                          .catch((err) =>
                            setMachinesMsg(err instanceof Error ? err.message : String(err)),
                          )
                          .finally(() => setMachinesBusy(false));
                      }}
                    >
                      重新注册本机
                    </button>
                  ) : null}
                </div>
                <div className="llm-list">
                  {machines.length === 0 ? (
                    <div className="muted small">暂无已注册电脑。请用桌面或移动客户端登录以登记。</div>
                  ) : (
                    machines.map((m) => (
                      <div className="llm-item" key={m.id}>
                        <div>
                          <div className="agent-name">
                            {m.label}{" "}
                            <span className="tag">{m.connected ? "可操作" : "未连接"}</span>
                          </div>
                          <div className="agent-desc">
                            {m.platform}
                            {m.os ? ` · ${m.os}` : ""}
                            {m.arch ? ` · ${m.arch}` : ""}
                            {m.app ? ` · ${m.app}` : ""}
                            {m.last_seen ? ` · 最近 ${m.last_seen}` : ""}
                          </div>
                        </div>
                        <div className="llm-actions">
                          <button
                            type="button"
                            className="danger"
                            disabled={machinesBusy}
                            onClick={() => {
                              setMachinesBusy(true);
                              void deleteMachine(m.id)
                                .then(() => {
                                  if (getStoredMachineId() === m.id) clearStoredMachineId();
                                  setMachinesMsg(`已删除 ${m.label}`);
                                  return refreshMachines();
                                })
                                .catch((err) =>
                                  setMachinesMsg(err instanceof Error ? err.message : String(err)),
                                )
                                .finally(() => setMachinesBusy(false));
                            }}
                          >
                            删除
                          </button>
                        </div>
                      </div>
                    ))
                  )}
                </div>
                {machinesMsg ? <div className="status">{machinesMsg}</div> : null}
              </>
            )}

            {settingsTab === "secrets" && (
              <div className="settings-panel">
                <h3>Bot 密钥</h3>
                <p className="muted">仅存元数据与密文；列表 API 永不返回明文。需配置 ENCRYPTION_KEY / BOT_SECRETS_KEY。</p>
                <form
                  className="stack"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void (async () => {
                      try {
                        await createBotSecret({
                          name: secretFormName,
                          value: secretFormValue,
                          origin: secretFormOrigin,
                          auth_type: "bearer",
                        });
                        setSecretFormValue("");
                        setSecretMsg("已保存");
                        await refreshSecrets();
                      } catch (err) {
                        setSecretMsg(err instanceof Error ? err.message : String(err));
                      }
                    })();
                  }}
                >
                  <label>
                    名称
                    <input value={secretFormName} onChange={(e) => setSecretFormName(e.target.value)} />
                  </label>
                  <label>
                    Origin (HTTPS)
                    <input value={secretFormOrigin} onChange={(e) => setSecretFormOrigin(e.target.value)} />
                  </label>
                  <label>
                    值
                    <input type="password" value={secretFormValue} onChange={(e) => setSecretFormValue(e.target.value)} />
                  </label>
                  <button type="submit" className="primary">
                    添加密钥
                  </button>
                </form>
                <ul className="list">
                  {botSecrets.map((s) => (
                    <li key={s.id}>
                      <strong>{s.name}</strong> · {s.origin || "(no origin)"} · {s.auth_type}
                      <button
                        type="button"
                        className="ghost"
                        onClick={() =>
                          void deleteBotSecret(s.id).then(() => refreshSecrets()).catch((e) => setSecretMsg(String(e)))
                        }
                      >
                        删除
                      </button>
                    </li>
                  ))}
                </ul>
                {secretRequests.length ? (
                  <div className="status">待处理请求：{secretRequests.length}</div>
                ) : null}
                {secretMsg ? <div className="status">{secretMsg}</div> : null}
              </div>
            )}
              </div>
            </div>
          </div>
        </div>
      )}
      <SecretPromptModal
        request={secretPrompt}
        onClose={() => setSecretPrompt(null)}
        onResolved={() => void refreshSecrets()}
      />
    </div>
  );
}
