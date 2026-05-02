import { useCallback, useState } from "react";
import type { ChatMessage, ChatMode, ChatSession } from "../types/chat";
import { generateId, getApiBaseUrl } from "../lib/utils";
import type { ThinkingStep } from "../components/agent/ThinkingCollapse";
import { generateSuggestions } from "../components/agent/SuggestionChips";
import type { Suggestion } from "../components/agent/SuggestionChips";

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

function buildThinkingSteps(
  mode: string,
  thoughts: string[],
  hasResponse: boolean,
): ThinkingStep[] {
  const baseSteps: ThinkingStep[] = [
    {
      id: "triage",
      label: "分析问题类型",
      status: "active",
      detail: "启动诊断...",
    },
    { id: "metrics", label: "拉取指标证据", status: "pending" },
    { id: "logs", label: "检索日志特征", status: "pending" },
    { id: "knowledge", label: "匹配历史案例", status: "pending" },
    { id: "reporter", label: "聚合生成结论", status: "pending" },
  ];

  if (!hasResponse && thoughts.length === 0) return baseSteps;

  // Mark steps based on thoughts and response
  const doneCount = hasResponse ? 5 : Math.min(thoughts.length + 1, 4);
  const activeIdx = hasResponse ? -1 : doneCount;

  return baseSteps.map((step, i) => {
    if (i < doneCount) return { ...step, status: "done" as const };
    if (i === activeIdx)
      return {
        ...step,
        status: "active" as const,
        detail: thoughts[i - 1] || "处理中...",
      };
    return step;
  });
}

// 工具名称到 thinking step id 的映射
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
  setThinkingSteps: React.Dispatch<React.SetStateAction<ThinkingStep[]>>,
  setStreamingThoughts: React.Dispatch<React.SetStateAction<string[]>>,
) {
  const { type, name, payload } = event;
  const toolName = payload?.tool_name || name || "";

  if (type === "tool_call_start") {
    const stepId = TOOL_STEP_MAP[toolName];
    const label = TOOL_LABELS[toolName] || toolName;

    if (stepId) {
      setThinkingSteps((prev) =>
        prev.map((s) =>
          s.id === stepId
            ? { ...s, status: "active" as const, detail: `正在${label}...` }
            : s,
        ),
      );
    }

    setStreamingThoughts((prev) => {
      const msg = `正在${label}...`;
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "tool_call_end") {
    const stepId = TOOL_STEP_MAP[toolName];
    const label = TOOL_LABELS[toolName] || toolName;
    const durationMs = payload?.duration_ms || 0;
    const success = payload?.success !== false;
    const error = payload?.error || "";
    const summary = payload?.summary || "";

    if (stepId) {
      if (success) {
        const detail =
          durationMs > 0 ? `${label}完成 (${durationMs}ms)` : `${label}完成`;
        setThinkingSteps((prev) =>
          prev.map((s) =>
            s.id === stepId ? { ...s, status: "done" as const, detail } : s,
          ),
        );
      } else {
        setThinkingSteps((prev) =>
          prev.map((s) =>
            s.id === stepId
              ? {
                  ...s,
                  status: "error" as const,
                  detail: `${label}失败: ${error}`,
                }
              : s,
          ),
        );
      }
    }

    // 记录到 thoughts
    setStreamingThoughts((prev) => {
      let msg: string;
      if (success) {
        msg =
          durationMs > 0 ? `${label}完成 (${durationMs}ms)` : `${label}完成`;
        if (summary) msg += `: ${summary}`;
      } else {
        msg = `${label}失败: ${error}`;
      }
      return prev.includes(msg) ? prev : [...prev, msg];
    });
  } else if (type === "model_start") {
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
    setThinkingSteps((prev) =>
      prev.map((s) =>
        s.status === "active"
          ? { ...s, status: "error" as const, detail: error }
          : s,
      ),
    );
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

      setMessages((prev) => [...prev, userMsg]);
      setIsLoading(true);
      setStreamingContent("");
      setStreamingThoughts([]);
      setSuggestions([]);
      setThinkingSteps(buildThinkingSteps(mode, [], false));

      const baseUrl = getApiBaseUrl();

      if (mode === "quick") {
        try {
          // Simulate thinking steps during quick mode
          setThinkingSteps((prev) =>
            prev.map((s, i) =>
              i === 0
                ? { ...s, status: "active" as const, detail: "识别查询意图..." }
                : s,
            ),
          );
          await new Promise((r) => setTimeout(r, 400));
          setThinkingSteps((prev) =>
            prev.map((s, i) =>
              i === 0
                ? { ...s, status: "done" as const }
                : i === 1
                  ? { ...s, status: "active" as const, detail: "检索指标..." }
                  : s,
            ),
          );

          // Mark remaining steps
          setThinkingSteps((prev) =>
            prev.map((s) => ({ ...s, status: "done" as const })),
          );
          const answer = await requestQuickAnswer({
            baseUrl,
            sessionId,
            question: trimmed,
            selectedSkillIds: options.selectedSkillIds || [],
          });
          const assistantMsg: ChatMessage = {
            id: generateId(),
            role: "assistant",
            content: answer,
            timestamp: Date.now(),
          };
          setMessages((prev) => [...prev, assistantMsg]);
          setSuggestions(generateSuggestions(answer, mode));
        } catch (err: any) {
          setThinkingSteps((prev) =>
            prev.map((s) =>
              s.status === "active"
                ? { ...s, status: "error" as const, detail: err?.message }
                : s,
            ),
          );
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: `请求失败: ${err?.message || "未知错误"}`,
              timestamp: Date.now(),
            },
          ]);
        } finally {
          setIsLoading(false);
          // Clear thinking steps after a delay so user can see completion
          setTimeout(() => setThinkingSteps([]), 2500);
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
        let thoughtCount = 0;
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
              // Update thinking to show reporter active
              if (thoughtCount > 0) {
                setThinkingSteps((prev) =>
                  prev.map((s) =>
                    s.id === "reporter"
                      ? {
                          ...s,
                          status: "active" as const,
                          detail: "生成诊断报告中...",
                        }
                      : s,
                  ),
                );
              }
            } else if (event === "agent_event") {
              const agentEvt = parseJsonSafe(data);
              if (agentEvt) {
                handleAgentEvent(
                  agentEvt,
                  setThinkingSteps,
                  setStreamingThoughts,
                );
              }
            } else if (event === "thought") {
              const thought = data.trim();
              if (thought) {
                thoughtCount++;
                setStreamingThoughts((prev) =>
                  prev.includes(thought) ? prev : [...prev, thought],
                );
                // Update thinking step
                const stepIdx = Math.min(thoughtCount, 3);
                const stepIds = ["triage", "metrics", "logs", "knowledge"];
                setThinkingSteps((prev) =>
                  prev.map((s) =>
                    s.id === stepIds[stepIdx - 1]
                      ? { ...s, status: "done" as const, detail: thought }
                      : s,
                  ),
                );
                if (stepIdx < 4) {
                  setThinkingSteps((prev) =>
                    prev.map((s) =>
                      s.id === stepIds[stepIdx]
                        ? {
                            ...s,
                            status: "active" as const,
                            detail: "处理中...",
                          }
                        : s,
                    ),
                  );
                }
              }
            } else if (event === "error") {
              setThinkingSteps((prev) =>
                prev.map((s) =>
                  s.status === "active"
                    ? { ...s, status: "error" as const, detail: data }
                    : s,
                ),
              );
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
          } else if (event === "done") {
            streamDone = true;
          }
        }

        setThinkingSteps((prev) =>
          prev.map((s) => ({ ...s, status: "done" as const })),
        );

        if (fullContent.trim()) {
          setMessages((prev) => [
            ...prev,
            {
              id: generateId(),
              role: "assistant",
              content: fullContent,
              timestamp: Date.now(),
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
            setThinkingSteps((prev) =>
              prev.map((s) => ({ ...s, status: "done" as const })),
            );
            setMessages((prev) => [
              ...prev,
              {
                id: generateId(),
                role: "assistant",
                content: fallbackAnswer,
                timestamp: Date.now(),
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

        setThinkingSteps((prev) =>
          prev.map((s) =>
            s.status === "active"
              ? { ...s, status: "error" as const, detail: err?.message }
              : s,
          ),
        );
        setMessages((prev) => {
          if (partialContent.trim()) {
            return [
              ...prev,
              {
                id: generateId(),
                role: "assistant",
                content: partialContent,
                timestamp: Date.now(),
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
            },
          ];
        });
      } finally {
        setIsLoading(false);
        setStreamingContent("");
        setStreamingThoughts([]);
        setAbortCtrl(null);
        // Clear thinking steps after delay
        setTimeout(() => setThinkingSteps([]), 2500);
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
