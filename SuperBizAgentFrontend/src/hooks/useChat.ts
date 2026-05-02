import { useCallback, useState, type Dispatch, type SetStateAction } from "react";
import type { ChatExecutionStep, ChatMessage, ChatMode, ChatSession } from "../types/chat";
import { generateId, getApiBaseUrl } from "../lib/utils";
import { generateSuggestions } from "../components/agent/SuggestionChips";
import type { Suggestion } from "../components/agent/SuggestionChips";

type ThinkingStep = ChatExecutionStep;
type SetThinkingSteps = Dispatch<SetStateAction<ThinkingStep[]>>;

interface SendOptions {
  selectedSkillIds?: string[];
}

interface QuickAnswerRequest {
  baseUrl: string;
  sessionId: string;
  question: string;
  selectedSkillIds: string[];
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
        return upsertStep(prev, stepId, label || "调用工具", {
          status: "done",
          detail,
        });
      }
      return upsertStep(prev, stepId, label || "调用工具", {
        status: "error",
        detail: `${label || "工具"}失败: ${error}`,
      });
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
  const [mode, setMode] = useState<ChatMode>("quick");
  const [sessionId, setSessionId] = useState(() => generateId());
  const [abortCtrl, setAbortCtrl] = useState<AbortController | null>(null);

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
            upsertStep(
              prev.map((s) =>
                s.id === "intent"
                  ? { ...s, status: "done" as const, detail: "请求已识别" }
                  : s,
              ),
              "reporter",
              "生成回复",
              { status: "active", detail: "组织回复..." },
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
            } else if (event === "thought") {
              const thought = data.trim();
              if (thought) {
                setStreamingThoughts((prev) =>
                  prev.includes(thought) ? prev : [...prev, thought],
                );
                const detail =
                  thought.length > 80 ? `${thought.slice(0, 80)}...` : thought;
                commitThinkingSteps((prev) =>
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
            );
          } else if (event === "done") {
            streamDone = true;
          }
        }

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
    setMode("quick");
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
    mode,
    sessionId,
    send,
    stop,
    newSession,
    loadSession,
    setMode,
    setMessages,
    clearSuggestions,
  };
}
