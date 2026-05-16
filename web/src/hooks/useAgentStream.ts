import { useCallback, useReducer, useRef } from "react";
import type {
  AgentEvent,
  AgentState,
  CriticVerdictEntry,
  PlanStep,
  ScreenshotEntry,
  StepStatus,
  ToolCallEntry,
} from "../types";

const initialState: AgentState = {
  status: "idle",
  runId: null,
  task: "",
  steps: [],
  stepStatus: {},
  currentStepId: null,
  planReason: "",
  replanCount: 0,
  toolCalls: [],
  verdicts: [],
  latestScreenshot: null,
  screenshots: [],
  vars: {},
  lastError: null,
};

type Action =
  | { kind: "reset" }
  | { kind: "start"; task: string }
  | { kind: "run_start"; runId: string; task: string }
  | { kind: "plan"; steps: PlanStep[]; reason: string; replanCount: number }
  | { kind: "step_start"; stepId: string }
  | { kind: "tool_call"; entry: ToolCallEntry }
  | { kind: "critic"; verdict: CriticVerdictEntry }
  | { kind: "screenshot"; shot: ScreenshotEntry }
  | { kind: "vars"; vars: Record<string, unknown> }
  | { kind: "done" }
  | { kind: "abort"; reason: string }
  | { kind: "error"; message: string };

function reducer(state: AgentState, a: Action): AgentState {
  switch (a.kind) {
    case "reset":
      return initialState;
    case "start":
      return { ...initialState, status: "running", task: a.task };
    case "run_start":
      return { ...state, runId: a.runId, task: a.task };
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
    case "screenshot":
      return {
        ...state,
        latestScreenshot: a.shot,
        screenshots: [...state.screenshots, a.shot],
      };
    case "vars":
      return { ...state, vars: a.vars };
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
  for (const block of chunk.split("\n\n")) {
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
      case "run_start":
        events.push({
          type: "run_start",
          run_id: p.run_id as string,
          task: p.task as string,
          config: p.config as AgentEvent extends { type: "run_start"; config: infer C } ? C : never,
          credentials_known: (p.credentials_known as string[]) ?? [],
        });
        break;
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
      case "screenshot":
        events.push({
          type: "screenshot",
          shot: {
            step_id: p.step_id as string,
            after_tool: p.after_tool as string,
            data_url: p.data_url as string,
            url: (p.url as string) ?? null,
            timestamp: Date.now(),
          },
        });
        break;
      case "vars_snapshot":
        events.push({
          type: "vars_snapshot",
          vars: (p.vars as Record<string, unknown>) ?? {},
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
      dispatch({ kind: "start", task });

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

      try {
        while (true) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lastBreak = buffer.lastIndexOf("\n\n");
          if (lastBreak === -1) continue;
          const ready = buffer.slice(0, lastBreak + 2);
          buffer = buffer.slice(lastBreak + 2);
          for (const ev of parseSseChunk(ready)) {
            applyEvent(ev, dispatch);
          }
        }
      } catch (e) {
        if ((e as Error).name !== "AbortError") {
          dispatch({ kind: "error", message: (e as Error).message });
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

function applyEvent(ev: AgentEvent, dispatch: React.Dispatch<Action>) {
  switch (ev.type) {
    case "run_start":
      dispatch({ kind: "run_start", runId: ev.run_id, task: ev.task });
      break;
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
    case "screenshot":
      dispatch({ kind: "screenshot", shot: ev.shot });
      break;
    case "vars_snapshot":
      dispatch({ kind: "vars", vars: ev.vars });
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
