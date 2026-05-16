export type StepStatus = "pending" | "running" | "succeeded" | "failed" | "replanned";

export interface PlanStep {
  id: string;
  goal: string;
  success_criteria?: string;
  depends_on?: string[];
}

export interface ToolCallEntry {
  step_id: string;
  iter: number;
  id: string;
  tool: string;
  params: unknown;
  result: unknown;
  executor_output?: string | null;
  timestamp: number;
}

export interface CriticVerdictEntry {
  step_id: string;
  succeeded: boolean;
  evidence: string;
  next_action: "continue" | "replan" | "abort";
  feedback: string;
  reason: string;
  timestamp: number;
}

export type AgentEvent =
  | { type: "plan"; steps: PlanStep[]; reason: string; replan_count: number }
  | { type: "step_start"; step_id: string; goal: string; success_criteria: string }
  | { type: "tool_call"; entry: ToolCallEntry }
  | { type: "critic"; verdict: CriticVerdictEntry }
  | { type: "router_idle"; step_id: string; reason: string }
  | { type: "done"; reason: string }
  | { type: "abort"; reason: string }
  | { type: "error"; message: string; where?: string }
  | { type: "close" };

export interface AgentState {
  status: "idle" | "running" | "done" | "aborted" | "error";
  steps: PlanStep[];
  stepStatus: Record<string, StepStatus>;
  currentStepId: string | null;
  planReason: string;
  replanCount: number;
  toolCalls: ToolCallEntry[];
  verdicts: CriticVerdictEntry[];
  lastError: string | null;
}
