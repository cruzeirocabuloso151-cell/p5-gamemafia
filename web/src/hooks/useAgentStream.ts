import { useCallback, useReducer, useRef } from "react";
import type {
  AgentEvent,
  AgentState,
  CriticVerdictEntry,
  PlanStep,
  StepStatus,
  ToolCallEntry,
} from "../types";

const initialState: AgentState = {
  status: "idle",
  steps: [],
  stepStatus: {},
  currentStepId: null,
  planReason: "",
  replanCount: 0,
  toolCalls: [],
  verdicts: [],
  lastError: null,
};

type Action =
  | { kind: "reset" }
  | { kind: "start" }
  | { kind: "plan"; steps: PlanStep[]; reason: string; replanCount: number }
  | { kind: "step_start"; stepId: string }
  | { kind: "tool_call"; entry: ToolCallEntry }
  | { kind: "critic"; verdict: CriticVerdictEntry }
  | { kind: "done" }
  | { kind: "abort"; reason: string }
  | { kind: "error"; message: string };

function reducer(state: AgentState, a: Action): AgentState {
  switch (a.kind) {
    case "reset":
      return initialState;
    case "start":
      return { ...initialState, status: "running" };
    case "plan": {
      const stepStatus: Record<string, StepStatus> = {};
      a.steps.forEach((s) => (stepStatus[s.id] = "pending"));
      return {
        ...state,
        status: "running",
        steps: a.steps,
        stepStatus,
        planReason: a.reason,
        replanCount: a.replanCount,
        currentStepId: null,
      };
    }
    case "step_start":
      return {
        ...state,
        currentStepId: a.stepId,
        stepStatus: { ...state.stepStatus, [a.stepId]: "running" },
      };
    case "tool_call":
      return { ...state, toolCalls: [...state.toolCalls, a.entry] };
    case "critic": {
      const newStatus: StepStatus =
        a.verdict.next_action === "replan"
          ? "replanned"
          : a.verdict.succeeded
            ? "succeeded"
            : "failed";
      return {
        ...state,
        verdicts: [...state.verdicts, a.verdict],
        stepStatus: { ...state.stepStatus, [a.verdict.step_id]: newStatus },
      };
    }
    case "done":
      return { ...state, status: "done", currentStepId: null };
    case "abort":
      return { ...state, status: "aborted", lastError: a.reason, currentStepId: null };
    case "error":
      return { ...state, status: "error", lastError: a.message, currentStepId: null };
    default:
      return state;
  }
}

function parseSseChunk(chunk: string): AgentEvent[] {
  const events: AgentEvent[] = [];
  const blocks = chunk.split("\n\n");
  for (const block of blocks) {
    if (!block.trim()) continue;
    let eventName = "";
    const dataLines: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith("event: ")) eventName = line.slice(7).trim();
      else if (line.startsWith("data: ")) dataLines.push(line.slice(6));
    }
    if (!eventName) continue;
    const raw = dataLines.join("\n");
    let parsed: unknown = {};
    try {
      parsed = raw ? JSON.parse(raw) : {};
    } catch {
      continue;
    }
    const p = parsed as Record<string, unknown>;
    switch (eventName) {
      case "plan":
        events.push({
          type: "plan",
          steps: p.steps as PlanStep[],
          reason: (p.reason as string) ?? "",
          replan_count: (p.replan_count as number) ?? 0,
        });
        break;
      case "step_start":
        events.push({
          type: "step_start",
          step_id: p.step_id as string,
          goal: p.goal as string,
          success_criteria: (p.success_criteria as string) ?? "",
        });
        break;
      case "tool_call":
        events.push({
          type: "tool_call",
          entry: { ...(p as unknown as ToolCallEntry), timestamp: Date.now() },
        });
        break;
      case "critic":
        events.push({
          type: "critic",
          verdict: { ...(p as unknown as CriticVerdictEntry), timestamp: Date.now() },
        });
        break;
      case "router_idle":
        events.push({
          type: "router_idle",
          step_id: p.step_id as string,
          reason: (p.reason as string) ?? "",
        });
        break;
      case "done":
        events.push({ type: "done", reason: (p.reason as string) ?? "" });
        break;
      case "abort":
        events.push({ type: "abort", reason: (p.reason as string) ?? "" });
        break;
      case "error":
        events.push({
          type: "error",
          message: (p.message as string) ?? "erro desconhecido",
          where: p.where as string | undefined,
        });
        break;
      case "close":
        events.push({ type: "close" });
        break;
    }
  }
  return events;
}

export function useAgentStream(endpoint: string) {
  const [state, dispatch] = useReducer(reducer, initialState);
  const abortRef = useRef<AbortController | null>(null);

  const run = useCallback(
    async (task: string) => {
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      dispatch({ kind: "start" });

      let resp: Response;
      try {
        resp = await fetch(endpoint, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ task }),
          signal: controller.signal,
        });
      } catch (e) {
        dispatch({ kind: "error", message: `falha ao conectar: ${(e as Error).message}` });
        return;
      }

      if (!resp.ok || !resp.body) {
        dispatch({ kind: "error", message: `HTTP ${resp.status}` });
        return;
      }

      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lastBreak = buffer.lastIndexOf("\n\n");
        if (lastBreak === -1) continue;
        const ready = buffer.slice(0, lastBreak + 2);
        buffer = buffer.slice(lastBreak + 2);

        for (const ev of parseSseChunk(ready)) {
          switch (ev.type) {
            case "plan":
              dispatch({
                kind: "plan",
                steps: ev.steps,
                reason: ev.reason,
                replanCount: ev.replan_count,
              });
              break;
            case "step_start":
              dispatch({ kind: "step_start", stepId: ev.step_id });
              break;
            case "tool_call":
              dispatch({ kind: "tool_call", entry: ev.entry });
              break;
            case "critic":
              dispatch({ kind: "critic", verdict: ev.verdict });
              break;
            case "done":
              dispatch({ kind: "done" });
              break;
            case "abort":
              dispatch({ kind: "abort", reason: ev.reason });
              break;
            case "error":
              dispatch({
                kind: "error",
                message: ev.where ? `${ev.where}: ${ev.message}` : ev.message,
              });
              break;
            case "router_idle":
            case "close":
              break;
          }
        }
      }
    },
    [endpoint],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    dispatch({ kind: "abort", reason: "cancelado pelo usuário" });
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    dispatch({ kind: "reset" });
  }, []);

  return { state, run, cancel, reset };
}
