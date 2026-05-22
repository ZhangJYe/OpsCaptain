import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import type { AIOpsEngine, ChatExecutionStep, ChatMessage, ChatMode, ChatSession } from "../types/chat";
import { generateId, getApiBaseUrl } from "../lib/utils";
import { generateSuggestions } from "../components/agent/SuggestionChips";
import type { Suggestion } from "../components/agent/SuggestionChips";

type ThinkingStep = ChatExecutionStep;
type SetThinkingSteps = Dispatch<SetStateAction<ThinkingStep[]>>;

interface SendOptions {
  selectedSkillIds?: string[];
}

interface AIOpsOptions {
  aiOpsEngine?: AIOpsEngine;
}

interface QuickAnswerRequest {
  baseUrl: string;
  sessionId: string;
  question: string;
  selectedSkillIds: string[];
}

interface AIOpsRequest {
  baseUrl: string;
  sessionId: string;
  query: string;
  engine: AIOpsEngine;
}

interface AIOpsPayload {
  result: string;
  trace_id?: string;
  detail?: string[];
  engine?: string;
  degraded?: boolean;
  degradation_reason?: string;
  confidence?: number;
  evidence?: Array<{ source_type: string; source_id: string; title: string; snippet: string; score: number; uri?: string }>;
  next_actions?: string[];
  started_at?: number;
  finished_at?: number;
}

function parseJsonSafe(raw: string): any {
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function normalizeResponsePayload(data: any): any {
  if (data && typeof data === "object" && "data" in data && data.data) {
    return data.data;
  }
  return data;
}

function extractAnswer(payload: any): string {
  const content = payload?.answer || payload?.content || payload?.message || "";
  return String(content || "").trim() || "无响应";
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function waitForStreamingReveal(content: string): Promise<void> {
  const length = Array.from(content || "").length;
  if (length === 0) return;
  const delay = Math.min(1800, Math.max(260, Math.ceil(length / 12) * 16));
  await wait(delay);
}

function parseSSEBlock(block: string): { event: string; data: string } {
  let event = "message";
  const dataLines: string[] = [];

  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      event = line.slice(6).trim() || "message";
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  return {
    event,
    data: dataLines.join("\n"),
  };
}

function pullSSEBlock(buffer: string): { block: string; rest: string } | null {
  const match = buffer.match(/\r?\n\r?\n/);
  if (!match || match.index === undefined) {
    return null;
  }
  const boundary = match.index;
  return {
    block: buffer.slice(0, boundary),
    rest: buffer.slice(boundary + match[0].length),
  };
}

async function requestQuickAnswer({
  baseUrl,
  sessionId,
  question,
  selectedSkillIds,
}: QuickAnswerRequest): Promise<string> {
  const res = await fetch(`${baseUrl}/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      Id: sessionId,
      Question: question,
      SelectedSkillIds: selectedSkillIds,
    }),
  });
  const data = await res.json();
  const payload = normalizeResponsePayload(data);
  if (!res.ok) {
    throw new Error(String(data?.message || `HTTP ${res.status}`));
  }
  return extractAnswer(payload);
}

async function requestAIOps({ baseUrl, sessionId, query, engine }: AIOpsRequest): Promise<AIOpsPayload> {
  const res = await fetch(`${baseUrl}/ai_ops`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      session_id: sessionId,
      query,
      engine,
    }),
  });
  const data = await res.json();
  const payload = normalizeResponsePayload(data);
  if (!res.ok) {
    throw new Error(String(data?.message || payload?.result || `HTTP ${res.status}`));
  }
  return {
    result: String(payload?.result || "").trim() || "无响应",
    trace_id: payload?.trace_id,
    detail: Array.isArray(payload?.detail) ? payload.detail : [],
    engine: payload?.engine,
    degraded: Boolean(payload?.degraded),
    degradation_reason: payload?.degradation_reason,
    confidence: typeof payload?.confidence === 'number' ? payload.confidence : undefined,
    evidence: Array.isArray(payload?.evidence) ? payload.evidence : undefined,
    next_actions: Array.isArray(payload?.next_actions) ? payload.next_actions : undefined,
    started_at: payload?.started_at,
    finished_at: payload?.finished_at,
  };
}

interface AIOpsRunInfo {
  trace_id: string;
  task_id: string;
  engine: string;
  status: string;
  degraded?: boolean;
  degradation_reason?: string;
  approval_required?: boolean;
  approval_request_id?: string;
}

interface AIOpsTraceEvent {
  event_id: string;
  task_id: string;
  trace_id: string;
  type: string;
  agent: string;
  message?: string;
  payload?: Record<string, any>;
  created_at: number;
}

interface AIOpsResultResponse {
  found: boolean;
  status?: string;
  trace_id: string;
  result?: string;
  detail?: string[];
  engine?: string;
  confidence?: number;
  evidence?: AIOpsPayload["evidence"];
  next_actions?: string[];
  degraded?: boolean;
  degradation_reason?: string;
  started_at?: number;
  finished_at?: number;
}

async function requestAIOpsRuns(baseUrl: string, query: string, engine: string, signal?: AbortSignal): Promise<AIOpsRunInfo> {
  const res = await fetch(`${baseUrl}/ai_ops_runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, engine }),
    signal,
  });
  const data = await res.json();
  const payload = normalizeResponsePayload(data);
  if (!res.ok) {
    throw new Error(String(data?.message || `HTTP ${res.status}`));
  }
  return {
    trace_id: payload.trace_id || "",
    task_id: payload.task_id || "",
    engine: payload.engine || engine,
    status: payload.status || "running",
    degraded: Boolean(payload?.degraded),
    degradation_reason: payload?.degradation_reason,
    approval_required: Boolean(payload?.approval_required),
    approval_request_id: payload?.approval_request_id,
  };
}

async function requestAIOpsTrace(baseUrl: string, traceId: string, signal?: AbortSignal): Promise<AIOpsTraceEvent[]> {
  const res = await fetch(`${baseUrl}/ai_ops_trace?trace_id=${encodeURIComponent(traceId)}`, { signal });
  const data = await res.json();
  const payload = normalizeResponsePayload(data);
  if (!res.ok) return [];
  return Array.isArray(payload?.events) ? payload.events : [];
}

async function requestAIOpsResult(baseUrl: string, traceId: string, signal?: AbortSignal): Promise<AIOpsResultResponse> {
  const res = await fetch(`${baseUrl}/ai_ops_result?trace_id=${encodeURIComponent(traceId)}`, { signal });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(String(data?.message || `HTTP ${res.status}`));
  }
  return normalizeResponsePayload(data);
}

function applyTraceEventToSteps(
  steps: ThinkingStep[],
  event: AIOpsTraceEvent,
  engine: string,
): ThinkingStep[] | null {
  const stage = event.payload?.stage as string | undefined;
  const isGoS = isGoSEngine(engine);

  if (isGoS) {
    const stageMap: Record<string, { stepId: string; detail: string }> = {
      ingest: { stepId: "gos:hypothesis", detail: "解析症状并建立候选假设..." },
      ingest_done: { stepId: "gos:hypothesis", detail: event.message || "候选假设已建立" },
      frontier_selected: { stepId: "gos:experts", detail: event.message || "选中 frontier" },
      expert_planned: { stepId: "gos:experts", detail: event.message || "调度专家" },
      evidence_attached: { stepId: "gos:evidence", detail: event.message || "挂载证据" },
      confidence_updated: { stepId: "gos:confidence", detail: event.message || "置信度更新" },
      fsm_decision: { stepId: "gos:confidence", detail: event.message || "FSM 决策" },
      report: { stepId: "gos:reporter", detail: "生成证据链报告..." },
    };

    const mapping = stageMap[stage || ""];
    if (!mapping) return null;

    return steps.map((step) => {
      if (step.id === mapping.stepId) {
        const metaItem = event.message && event.message !== step.detail ? event.message : undefined;
        return {
          ...step,
          status: "active" as const,
          detail: mapping.detail,
          meta: metaItem ? [...(step.meta || []).slice(-2), metaItem] : step.meta,
        };
      }
      if (isStepBefore(step.id, mapping.stepId, isGoS) && step.status === "active") {
        return { ...step, status: "done" as const };
      }
      return step;
    });
  }

  if (event.type === "task_info" && event.message) {
    const infoCount = steps.filter((s) => s.id.startsWith("info:")).length;
    const infoId = `info:${infoCount}`;
    let next = upsertStep(steps, infoId, event.message.slice(0, 60), {
      status: "done",
      detail: event.message,
    });
    next = upsertStep(next, "evidence", "收集证据", {
      status: "active",
      detail: `已收集 ${infoCount + 1} 条执行细节`,
    });
    return next;
  }

  return null;
}

function isStepBefore(currentId: string, targetId: string, isGoS: boolean): boolean {
  if (isGoS) {
    const order = ["gos:hypothesis", "gos:experts", "gos:evidence", "gos:confidence", "gos:reporter"];
    return order.indexOf(currentId) < order.indexOf(targetId);
  }
  const order = ["engine", "dispatch", "evidence", "reporter"];
  return order.indexOf(currentId) < order.indexOf(targetId);
}

export function isGoSEngine(engine: AIOpsEngine | string | null | undefined): boolean {
  return engine === "gos_engine" || engine === "gos" || engine === "aiops_gos_engine";
}

function aiOpsEngineLabel(engine: AIOpsEngine | string | undefined): string {
  return isGoSEngine(engine) ? "GoS Belief" : "Plan-Execute";
}

function buildExecutionSteps(): ThinkingStep[] {
  return [
    {
      id: "intent",
      label: "理解请求",
      status: "active",
      detail: "识别查询意图...",
    },
    { id: "context", label: "装配上下文", status: "pending" },
    { id: "metrics", label: "拉取指标证据", status: "pending" },
    { id: "logs", label: "检索日志特征", status: "pending" },
    { id: "knowledge", label: "检索知识与案例", status: "pending" },
    { id: "reporter", label: "生成回复", status: "pending" },
  ];
}

function buildAIOpsSteps(engine: AIOpsEngine): ThinkingStep[] {
  if (isGoSEngine(engine)) {
    return [
      {
        id: "gos:hypothesis",
        label: "建立候选假设",
        status: "active",
        detail: "从症状抽取故障方向",
      },
      { id: "gos:experts", label: "调度专家检索", status: "pending" },
      { id: "gos:evidence", label: "挂载支持证据", status: "pending" },
      { id: "gos:confidence", label: "校准置信度", status: "pending" },
      { id: "gos:reporter", label: "生成证据链报告", status: "pending" },
    ];
  }

  return [
    {
      id: "engine",
      label: "选择引擎",
      status: "active",
      detail: aiOpsEngineLabel(engine),
    },
    { id: "dispatch", label: "调度 Runtime", status: "pending" },
    { id: "evidence", label: "收集证据", status: "pending" },
    { id: "reporter", label: "生成报告", status: "pending" },
  ];
}

function activateAIOpsSteps(steps: ThinkingStep[], engine: AIOpsEngine): ThinkingStep[] {
  if (isGoSEngine(engine)) {
    return upsertStep(
      steps.map((step) =>
        step.id === "gos:hypothesis"
          ? { ...step, status: "done" as const, detail: "候选假设已建立" }
          : step,
      ),
      "gos:experts",
      "调度专家检索",
      { status: "active", detail: "查询日志、知识库与 RAG 证据" },
    );
  }

  return upsertStep(
    steps.map((step) =>
      step.id === "engine"
        ? { ...step, status: "done" as const, detail: aiOpsEngineLabel(engine) }
        : step,
    ),
    "dispatch",
    "调度 Runtime",
    { status: "active", detail: "提交 AIOps 诊断任务..." },
  );
}

function completeAIOpsSteps(
  steps: ThinkingStep[],
  actualEngine: AIOpsEngine | string | undefined,
  payload: AIOpsPayload,
): ThinkingStep[] {
  const detailItems = payload.detail || [];
  const reportDetail =
    payload.degraded && payload.degradation_reason
      ? `降级完成：${payload.degradation_reason}`
      : "诊断报告已生成";

  if (isGoSEngine(actualEngine)) {
    const evidenceCount = payload.evidence?.length ?? 0;
    const confidencePct = payload.confidence != null ? `${Math.round(payload.confidence * 100)}%` : undefined;

    let next = steps.map((step) => {
      if (step.id === "gos:hypothesis") {
        return { ...step, status: "done" as const, detail: "候选故障方向已建立" };
      }
      if (step.id === "gos:experts") {
        return {
          ...step,
          status: "done" as const,
          detail: payload.trace_id ? `trace ${payload.trace_id}` : "专家链路已返回",
        };
      }
      if (step.id === "gos:evidence") {
        const count = evidenceCount > 0 ? `${evidenceCount} 条证据` : detailItems.length > 0 ? `${detailItems.length} 条 trace/detail` : "证据链已写入信念图";
        return { ...step, status: "done" as const, detail: count };
      }
      if (step.id === "gos:confidence") {
        const detail = payload.degraded
          ? "降级路径已记录"
          : confidencePct
          ? `置信度 ${confidencePct}，frontier 已收敛`
          : "frontier 已收敛";
        return { ...step, status: "done" as const, detail };
      }
      if (step.id === "gos:reporter") {
        return { ...step, status: payload.degraded ? "error" as const : "done" as const, detail: reportDetail };
      }
      return step;
    });

    if (payload.evidence) {
      for (const ev of payload.evidence.slice(0, 3)) {
        next = appendStepMeta(next, "gos:evidence", "挂载支持证据", `[${ev.source_type}] ${ev.title || ev.snippet || ev.source_id}`);
      }
    } else {
      for (const item of detailItems.slice(0, 3)) {
        next = appendStepMeta(next, "gos:evidence", "挂载支持证据", item);
      }
    }
    return next;
  }

  return upsertStep(
    upsertStep(
      steps.map((step) =>
        step.id === "engine"
          ? { ...step, status: "done" as const, detail: aiOpsEngineLabel(actualEngine) }
          : step.id === "dispatch"
          ? {
              ...step,
              status: "done" as const,
              detail: payload.trace_id ? `trace ${payload.trace_id}` : "Runtime 已返回",
            }
          : step,
      ),
      "evidence",
      "收集证据",
      {
        status: "done",
        detail: detailItems.length > 0 ? `${detailItems.length} 条执行细节` : aiOpsEngineLabel(actualEngine),
      },
    ),
    "reporter",
    "生成报告",
    { status: payload.degraded ? "error" : "done", detail: reportDetail },
  );
}

function visibleExecutionSteps(steps: ThinkingStep[]): ThinkingStep[] {
  return steps.filter((step) => step.status !== "pending");
}

function upsertStep(
  steps: ThinkingStep[],
  id: string,
  fallbackLabel: string,
  patch: Partial<ThinkingStep>,
): ThinkingStep[] {
  const exists = steps.some((step) => step.id === id);
  if (!exists) {
    return [...steps, { id, label: fallbackLabel, status: "pending", ...patch }];
  }
  return steps.map((step) => (step.id === id ? { ...step, ...patch } : step));
}

function appendStepMeta(
  steps: ThinkingStep[],
  id: string,
  fallbackLabel: string,
  message: string,
): ThinkingStep[] {
  const normalized = message.trim();
  if (!normalized) return steps;

  return upsertStep(steps, id, fallbackLabel, {}).map((step) => {
    if (step.id !== id) return step;
    const previous = step.meta || [];
    if (previous.includes(normalized)) return step;
    return { ...step, meta: [...previous.slice(-2), normalized] };
  });
}

function completeExecutionSteps(steps: ThinkingStep[]): ThinkingStep[] {
  let hasVisibleReporter = false;
  const next = steps.map((step) => {
    if (step.id === "intent" && step.status === "active") {
      return { ...step, status: "done" as const, detail: "请求已识别" };
    }
    if (step.id === "reporter" && step.status !== "pending") {
      hasVisibleReporter = true;
      return { ...step, status: "done" as const, detail: step.detail || "回复已生成" };
    }
    if (step.status === "active") {
      return { ...step, status: "done" as const };
    }
    return step;
  });

  if (hasVisibleReporter) {
    return next;
  }
  return upsertStep(next, "reporter", "生成回复", {
    status: "done",
    detail: "回复已生成",
  });
}

function markActiveAsError(steps: ThinkingStep[], detail?: string): ThinkingStep[] {
  return steps.map((step) =>
    step.status === "active"
      ? { ...step, status: "error" as const, detail: detail || "执行失败" }
      : step,
  );
}

// 工具名称到执行步骤的映射
const TOOL_STEP_MAP: Record<string, string> = {
  query_metrics: "metrics",
  query_prometheus: "metrics",
  query_alerts: "metrics",
  query_logs: "logs",
  search_logs: "logs",
  query_internal_docs: "knowledge",
  search_knowledge: "knowledge",
  rag_search: "knowledge",
};

// 已知工具标签（用于显示友好名称）
const TOOL_LABELS: Record<string, string> = {
  query_metrics: "查询指标",
  query_prometheus: "查询 Prometheus",
  query_alerts: "查询告警",
  query_logs: "查询日志",
  search_logs: "搜索日志",
  query_internal_docs: "检索知识库",
  search_knowledge: "搜索知识库",
  rag_search: "RAG 检索",
};

function handleAgentEvent(
  event: { type: string; name?: string; payload?: Record<string, any> },
  setThinkingSteps: SetThinkingSteps,
  setStreamingThoughts: Dispatch<SetStateAction<string[]>>,
) {
  const { type, name, payload } = event;
  const toolName = payload?.tool_name || name || "";

  if (type === "tool_call_start") {
    const stepId = TOOL_STEP_MAP[toolName] || `tool:${toolName || "unknown"}`;
    const label = TOOL_LABELS[toolName] || toolName;

    setThinkingSteps((prev) =>
      appendStepMeta(
        upsertStep(
          prev.map((s) =>
            s.id === "intent" && s.status === "active"
              ? { ...s, status: "done" as const, detail: "请求已识别" }
              : s,
          ),
          stepId,
          label || "调用工具",
          { status: "active", detail: `正在${label || "调用工具"}...` },
        ),
        stepId,
        label || "调用工具",
        "开始调用工具",
      ),
    );

    setStreamingThoughts((prev) => {
      const msg = `正在${label}...`;
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "tool_call_end") {
    const stepId = TOOL_STEP_MAP[toolName] || `tool:${toolName || "unknown"}`;
    const label = TOOL_LABELS[toolName] || toolName;
    const durationMs = payload?.duration_ms || 0;
    const success = payload?.success !== false;
    const error = payload?.error || "";

    setThinkingSteps((prev) => {
      if (success) {
        const detail =
          durationMs > 0 ? `${label || "工具"}完成 (${durationMs}ms)` : `${label || "工具"}完成`;
        return appendStepMeta(
          upsertStep(prev, stepId, label || "调用工具", {
            status: "done",
            detail,
          }),
          stepId,
          label || "调用工具",
          durationMs > 0 ? `返回成功，用时 ${durationMs}ms` : "返回成功",
        );
      }
      return appendStepMeta(
        upsertStep(prev, stepId, label || "调用工具", {
          status: "error",
          detail: `${label || "工具"}失败: ${error}`,
        }),
        stepId,
        label || "调用工具",
        "工具调用失败，已进入降级路径",
      );
    });

    setStreamingThoughts((prev) => {
      let msg: string;
      if (success) {
        msg =
          durationMs > 0 ? `${label}完成 (${durationMs}ms)` : `${label}完成`;
      } else {
        msg = `${label}失败: ${error}`;
      }
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "model_start") {
    setThinkingSteps((prev) =>
      upsertStep(
        prev.map((s) =>
          s.id === "intent" && s.status === "active"
            ? { ...s, status: "done" as const, detail: "请求已识别" }
            : s,
        ),
        "reporter",
        "生成回复",
        { status: "active", detail: "组织回复..." },
      ),
    );
    setStreamingThoughts((prev) => {
      const msg = "模型推理中...";
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "model_end") {
    const durationMs = payload?.duration_ms || 0;
    const totalTokens = payload?.total_tokens || 0;
    setThinkingSteps((prev) => {
      let detail = "模型输出完成";
      const metaParts: string[] = [];
      if (durationMs > 0) metaParts.push(`模型耗时 ${durationMs}ms`);
      if (totalTokens > 0) metaParts.push(`${totalTokens} tokens`);
      return appendStepMeta(
        upsertStep(prev, "reporter", "生成回复", {
          status: "done",
          detail,
        }),
        "reporter",
        "生成回复",
        metaParts.join(" · "),
      );
    });
    setStreamingThoughts((prev) => {
      let msg = "模型推理完成";
      if (durationMs > 0) msg += ` (${durationMs}ms`;
      if (totalTokens > 0) msg += `, ${totalTokens} tokens`;
      if (durationMs > 0 || totalTokens > 0) msg += ")";
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "error") {
    const error = payload?.error || "未知错误";
    setThinkingSteps((prev) => markActiveAsError(prev, error));
  }
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [streamingContent, setStreamingContent] = useState("");
  const [streamingThoughts, setStreamingThoughts] = useState<string[]>([]);
  const [thinkingSteps, setThinkingSteps] = useState<ThinkingStep[]>([]);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const [mode, setMode] = useState<ChatMode>("stream");
  const [sessionId, setSessionId] = useState(() => generateId());
  const [abortCtrl, setAbortCtrl] = useState<AbortController | null>(null);
  const [loadingEngine, setLoadingEngine] = useState<string | null>(null);

  const send = useCallback(
    async (query: string, options: SendOptions = {}) => {
      const trimmed = String(query || "").trim();
      if (!trimmed || isLoading) return;

      const userMsg: ChatMessage = {
        id: generateId(),
        role: "user",
        content: trimmed,
        timestamp: Date.now(),
      };
      let liveSteps = buildExecutionSteps();
      const commitThinkingSteps: SetThinkingSteps = (update) => {
        liveSteps =
          typeof update === "function"
            ? (update as (prev: ThinkingStep[]) => ThinkingStep[])(liveSteps)
            : update;
        setThinkingSteps(liveSteps);
      };

      setMessages((prev) => [...prev, userMsg]);
      setIsLoading(true);
      setStreamingContent("");
      setStreamingThoughts([]);
      setSuggestions([]);
      commitThinkingSteps(liveSteps);

      const baseUrl = getApiBaseUrl();

      if (mode === "quick") {
        try {
          commitThinkingSteps((prev) =>
            appendStepMeta(
              upsertStep(
                appendStepMeta(
                  prev.map((s) =>
                    s.id === "intent"
                      ? { ...s, status: "done" as const, detail: "请求已识别" }
                      : s,
                  ),
                  "intent",
                  "理解请求",
                  "进入快速回答链路",
                ),
                "reporter",
                "生成回复",
                { status: "active", detail: "组织回复..." },
              ),
              "reporter",
              "生成回复",
              "等待模型生成回答",
            ),
          );
          const answer = await requestQuickAnswer({
            baseUrl,
            sessionId,
            question: trimmed,
            selectedSkillIds: options.selectedSkillIds || [],
          });
          commitThinkingSteps(completeExecutionSteps(liveSteps));
          const assistantMsg: ChatMessage = {
            id: generateId(),
            role: "assistant",
            content: answer,
            timestamp: Date.now(),
            executionSteps: visibleExecutionSteps(liveSteps),
          };
          setMessages((prev) => [...prev, assistantMsg]);
          setSuggestions(generateSuggestions(answer, mode));
        } catch (err: any) {
          commitThinkingSteps((prev) => markActiveAsError(prev, err?.message));
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: `请求失败: ${err?.message || "未知错误"}`,
              timestamp: Date.now(),
              executionSteps: visibleExecutionSteps(liveSteps),
            },
          ]);
        } finally {
          setIsLoading(false);
        }
        return;
      }

      // Stream mode
      const ctrl = new AbortController();
      setAbortCtrl(ctrl);
      let partialContent = "";
      const selectedSkillIds = options.selectedSkillIds || [];

      try {
        const res = await fetch(`${baseUrl}/chat_stream`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            Id: sessionId,
            Question: trimmed,
            SelectedSkillIds: selectedSkillIds,
          }),
          signal: ctrl.signal,
        });

        if (!res.ok) {
          const text = await res.text();
          const maybeJson = parseJsonSafe(text);
          throw new Error(
            String(maybeJson?.message || text || `HTTP ${res.status}`),
          );
        }

        const reader = res.body?.getReader();
        if (!reader) throw new Error("No response body");

        const decoder = new TextDecoder();
        let buffer = "";
        let fullContent = "";
        let streamDone = false;

        while (true) {
          const { done, value } = await reader.read();
          if (value) {
            buffer += decoder.decode(value, { stream: !done });
          }

          let parsedBlock = pullSSEBlock(buffer);
          while (parsedBlock) {
            const block = parsedBlock.block;
            buffer = parsedBlock.rest;
            const { event, data } = parseSSEBlock(block);

            if (event === "message") {
              fullContent += data;
              partialContent = fullContent;
              setStreamingContent(fullContent);
              commitThinkingSteps((prev) =>
                appendStepMeta(
                  upsertStep(
                    prev.map((s) =>
                      s.id === "intent" && s.status === "active"
                        ? { ...s, status: "done" as const, detail: "请求已识别" }
                        : s,
                    ),
                    "reporter",
                    "生成回复",
                    { status: "active", detail: "生成回复中..." },
                  ),
                  "reporter",
                  "生成回复",
                  "开始向前端流式输出",
                ),
              );
            } else if (event === "agent_event") {
              const agentEvt = parseJsonSafe(data);
              if (agentEvt) {
                handleAgentEvent(
                  agentEvt,
                  commitThinkingSteps,
                  setStreamingThoughts,
                );
              }
            } else if (event === "contract_violation") {
              // Contract 阻断模式：响应可能不可靠
              const violationSummary = data.trim();
              if (violationSummary) {
                commitThinkingSteps((prev) =>
                  appendStepMeta(
                    upsertStep(
                      prev,
                      "contract",
                      "Contract 校验",
                      { status: "error", detail: "输出存在合规问题" },
                    ),
                    "contract",
                    "Contract 校验",
                    violationSummary,
                  ),
                );
                setStreamingThoughts((prev) => [
                  ...prev,
                  `⚠️ 输出存在合规问题: ${violationSummary}`,
                ]);
              }
            } else if (event === "schema_gate") {
              // Schema Gate 校验失败
              const schemaSummary = data.trim();
              if (schemaSummary) {
                commitThinkingSteps((prev) =>
                  appendStepMeta(
                    upsertStep(
                      prev,
                      "schema",
                      "Schema 校验",
                      { status: "error", detail: "输出质量不达标" },
                    ),
                    "schema",
                    "Schema 校验",
                    schemaSummary,
                  ),
                );
              }
            } else if (event === "thought") {
              const thought = data.trim();
              if (thought) {
                setStreamingThoughts((prev) =>
                  prev.includes(thought) ? prev : [...prev, thought],
                );
                const detail =
                  thought.length > 80 ? `${thought.slice(0, 80)}...` : thought;
                commitThinkingSteps((prev) =>
                  appendStepMeta(
                    upsertStep(
                      prev.map((s) =>
                        s.id === "intent" && s.status === "active"
                          ? { ...s, status: "done" as const, detail: "请求已识别" }
                          : s,
                      ),
                      "context",
                      "装配上下文",
                      { status: "done", detail },
                    ),
                    "context",
                    "装配上下文",
                    "history / memory / docs 已进入请求上下文",
                  ),
                );
              }
            } else if (event === "error") {
              commitThinkingSteps((prev) => markActiveAsError(prev, data));
              throw new Error(data || "流式请求失败");
            } else if (event === "done") {
              streamDone = true;
              break;
            }

            parsedBlock = pullSSEBlock(buffer);
          }

          if (streamDone) {
            break;
          }

          if (done) {
            break;
          }
        }

        if (buffer.trim()) {
          const { event, data } = parseSSEBlock(buffer);
          if (event === "message") {
            fullContent += data;
            partialContent = fullContent;
            setStreamingContent(fullContent);
            commitThinkingSteps((prev) =>
              appendStepMeta(
                upsertStep(
                  prev.map((s) =>
                    s.id === "intent" && s.status === "active"
                      ? { ...s, status: "done" as const, detail: "请求已识别" }
                      : s,
                  ),
                  "reporter",
                  "生成回复",
                  { status: "active", detail: "生成回复中..." },
                ),
                "reporter",
                "生成回复",
                "开始向前端流式输出",
              ),
            );
          } else if (event === "done") {
            streamDone = true;
          }
        }

        await waitForStreamingReveal(fullContent);
        commitThinkingSteps(completeExecutionSteps(liveSteps));
        if (fullContent.trim()) {
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: fullContent,
              timestamp: Date.now(),
              executionSteps: visibleExecutionSteps(liveSteps),
            },
          ]);
          setSuggestions(generateSuggestions(fullContent, mode));
        }
      } catch (err: any) {
        const isAbort = err?.name === "AbortError";
        let recoveredWithQuickFallback = false;

        if (!isAbort && !partialContent.trim()) {
          try {
            const fallbackAnswer = await requestQuickAnswer({
              baseUrl,
              sessionId,
              question: trimmed,
              selectedSkillIds,
            });
            recoveredWithQuickFallback = true;
            commitThinkingSteps(completeExecutionSteps(liveSteps));
            setMessages((prev) => [
              ...prev,
              {
                id: generateId(),
                role: "assistant",
                content: fallbackAnswer,
                timestamp: Date.now(),
                executionSteps: visibleExecutionSteps(liveSteps),
              },
            ]);
            setSuggestions(generateSuggestions(fallbackAnswer, "quick"));
          } catch (fallbackErr: any) {
            err = fallbackErr;
          }
        }

        if (recoveredWithQuickFallback) {
          return;
        }

        commitThinkingSteps((prev) => markActiveAsError(prev, err?.message));
        setMessages((prev) => {
          if (partialContent.trim()) {
            return [
              ...prev,
              {
                id: generateId(),
                role: "assistant",
                content: partialContent,
                timestamp: Date.now(),
                executionSteps: visibleExecutionSteps(liveSteps),
              },
            ];
          }
          if (isAbort) return prev;
          return [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: `流式请求失败: ${err?.message || "未知错误"}`,
              timestamp: Date.now(),
              executionSteps: visibleExecutionSteps(liveSteps),
            },
          ];
        });
      } finally {
        setIsLoading(false);
        setStreamingContent("");
        setStreamingThoughts([]);
        setAbortCtrl(null);
      }
    },
    [isLoading, mode, sessionId],
  );

  const sendAIOps = useCallback(
    async (query: string, options: AIOpsOptions = {}) => {
      const trimmed = String(query || "").trim();
      if (!trimmed || isLoading) return;

      const engine = options.aiOpsEngine || "plan_execute_replan";
      const userMsg: ChatMessage = {
        id: generateId(),
        role: "user",
        content: trimmed,
        timestamp: Date.now(),
      };

      let liveSteps = buildAIOpsSteps(engine);
      const commitThinkingSteps: SetThinkingSteps = (update) => {
        liveSteps =
          typeof update === "function"
            ? (update as (prev: ThinkingStep[]) => ThinkingStep[])(liveSteps)
            : update;
        setThinkingSteps(liveSteps);
      };

      setMessages((prev) => [...prev, userMsg]);
      setIsLoading(true);
      setLoadingEngine(engine);
      setStreamingContent("");
      setStreamingThoughts([`AIOps 使用 ${aiOpsEngineLabel(engine)} 引擎`]);
      setSuggestions([]);
      commitThinkingSteps(liveSteps);

      let pollTimer: ReturnType<typeof setInterval> | null = null;
      let pollTimeout: ReturnType<typeof setTimeout> | null = null;
      let rejectPolling: ((err: Error) => void) | null = null;
      let aborted = false;
      const pollAbort = new AbortController();
      setAbortCtrl(pollAbort);

      const cleanup = () => {
        if (pollTimer) {
          clearInterval(pollTimer);
          pollTimer = null;
        }
        if (pollTimeout) {
          clearTimeout(pollTimeout);
          pollTimeout = null;
        }
      };

      pollAbort.signal.addEventListener("abort", () => {
        aborted = true;
        cleanup();
        if (rejectPolling) {
          const reject = rejectPolling;
          rejectPolling = null;
          reject(new Error("已停止"));
        }
      });

      try {
        const baseUrl = getApiBaseUrl();
        commitThinkingSteps((prev) => activateAIOpsSteps(prev, engine));

        const runInfo = await requestAIOpsRuns(baseUrl, trimmed, engine, pollAbort.signal);
        const actualEngine = runInfo.engine || engine;

        if (runInfo.status !== "running" || !runInfo.trace_id) {
          const terminalContent = runInfo.degraded
            ? `AIOps 已降级: ${runInfo.degradation_reason || "服务暂时不可用"}`
            : runInfo.approval_required
            ? `请求已进入审批队列 (ID: ${runInfo.approval_request_id || "unknown"})`
            : "AIOps 无法启动";
          commitThinkingSteps((prev) => markActiveAsError(prev, terminalContent));
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: terminalContent,
              timestamp: Date.now(),
              executionSteps: visibleExecutionSteps(liveSteps),
              engine: actualEngine,
            },
          ]);
          return;
        }

        let seenEventIds = new Set<string>();

        const POLL_TIMEOUT_MS = 120_000;
        await new Promise<void>((resolve, reject) => {
          rejectPolling = reject;
          pollTimeout = setTimeout(() => {
            cleanup();
            rejectPolling = null;
            reject(new Error("AIOps 轮询超时（120s）"));
          }, POLL_TIMEOUT_MS);

          const poll = async () => {
            if (aborted) {
              cleanup();
              rejectPolling = null;
              reject(new Error("已停止"));
              return;
            }
            try {
              const events = await requestAIOpsTrace(baseUrl, runInfo.trace_id, pollAbort.signal);
              for (const event of events) {
                if (seenEventIds.has(event.event_id)) continue;
                seenEventIds.add(event.event_id);

                const updated = applyTraceEventToSteps(liveSteps, event, actualEngine);
                if (updated) {
                  commitThinkingSteps(updated);
                }

                if (event.type === "task_completed") {
                  cleanup();
                  rejectPolling = null;
                  resolve();
                  return;
                }
                if (event.type === "task_failed" || event.type === "task_timeout") {
                  cleanup();
                  rejectPolling = null;
                  reject(new Error(event.message || "AIOps 执行失败"));
                  return;
                }
              }
            } catch (pollErr) {
              if (aborted) {
                cleanup();
                rejectPolling = null;
                reject(new Error("已停止"));
                return;
              }
            }
          };

          poll();
          pollTimer = setInterval(poll, 1500);
        });

        const resultRes = await requestAIOpsResult(baseUrl, runInfo.trace_id, pollAbort.signal);
        if (!resultRes.found) {
          throw new Error("AIOps 执行完成但未找到结果");
        }

        if (resultRes.status === "failed") {
          throw new Error(resultRes.result || "AIOps 执行失败");
        }

        const payload: AIOpsPayload = {
          result: resultRes.result || "无响应",
          trace_id: runInfo.trace_id,
          detail: resultRes.detail,
          engine: resultRes.engine,
          degraded: resultRes.degraded,
          degradation_reason: resultRes.degradation_reason,
          confidence: resultRes.confidence,
          evidence: resultRes.evidence,
          next_actions: resultRes.next_actions,
          started_at: resultRes.started_at,
          finished_at: resultRes.finished_at,
        };

        commitThinkingSteps((prev) => completeAIOpsSteps(prev, actualEngine, payload));

        const assistantMsg: ChatMessage = {
          id: generateId(),
          role: "assistant",
          content: payload.result,
          timestamp: Date.now(),
          executionSteps: visibleExecutionSteps(liveSteps),
          engine: actualEngine,
          confidence: payload.confidence,
          evidenceCount: payload.evidence?.length,
          nextActions: payload.next_actions,
          startedAt: payload.started_at,
          finishedAt: payload.finished_at,
        };
        setMessages((prev) => [...prev, assistantMsg]);
        setSuggestions(generateSuggestions(payload.result, mode));
      } catch (err: any) {
        aborted = true;
        cleanup();
        const errorMessage = pollAbort.signal.aborted ? "已停止" : err?.message;
        commitThinkingSteps((prev) => markActiveAsError(prev, errorMessage));
        setMessages((prev) => [
          ...prev,
          {
            id: generateId(),
            role: "assistant",
            content: `AIOps 请求失败: ${errorMessage || "未知错误"}`,
            timestamp: Date.now(),
            executionSteps: visibleExecutionSteps(liveSteps),
            engine,
          },
        ]);
      } finally {
        cleanup();
        setIsLoading(false);
        setLoadingEngine(null);
        setAbortCtrl(null);
        setStreamingContent("");
        setStreamingThoughts([]);
      }
    },
    [isLoading, mode, sessionId],
  );

  const stop = useCallback(() => {
    abortCtrl?.abort();
    setIsLoading(false);
    setAbortCtrl(null);
    setThinkingSteps((prev) =>
      prev.map((s) =>
        s.status === "active"
          ? { ...s, status: "error" as const, detail: "用户中止" }
          : s,
      ),
    );
  }, [abortCtrl]);

  const newSession = useCallback(() => {
    if (isLoading) return false;
    setMessages([]);
    setStreamingContent("");
    setStreamingThoughts([]);
    setThinkingSteps([]);
    setSuggestions([]);
    setMode("stream");
    setSessionId(generateId());
    return true;
  }, [isLoading]);

  const loadSession = useCallback(
    (session: ChatSession) => {
      if (isLoading || !session) return false;
      setSessionId(session.id);
      setMessages(Array.isArray(session.messages) ? session.messages : []);
      setMode(session.mode === "stream" ? "stream" : "quick");
      setStreamingContent("");
      setStreamingThoughts([]);
      setThinkingSteps([]);
      setSuggestions([]);
      return true;
    },
    [isLoading],
  );

  const clearSuggestions = useCallback(() => setSuggestions([]), []);

  return {
    messages,
    streamingContent,
    streamingThoughts,
    thinkingSteps,
    suggestions,
    isLoading,
    loadingEngine,
    mode,
    sessionId,
    send,
    sendAIOps,
    stop,
    newSession,
    loadSession,
    setMode,
    setMessages,
    clearSuggestions,
  };
}
