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

export interface ScreenshotEntry {
  step_id: string;
  after_tool: string;
  data_url: string;
  url: string | null;
  timestamp: number;
}

export interface RuntimeConfigDTO {
  max_step_iters: number;
  max_replans: number;
  router_temperature: number;
  executor_temperature: number;
  screenshot_max_width: number;
  enable_browser: boolean;
}

export type AgentEvent =
  | { type: "run_start"; run_id: string; task: string; config: RuntimeConfigDTO; credentials_known: string[] }
  | { type: "plan"; steps: PlanStep[]; reason: string; replan_count: number }
  | { type: "step_start"; step_id: string; goal: string; success_criteria: string }
  | { type: "tool_call"; entry: ToolCallEntry }
  | { type: "critic"; verdict: CriticVerdictEntry }
  | { type: "screenshot"; shot: ScreenshotEntry }
  | { type: "router_idle"; step_id: string; reason: string }
  | { type: "vars_snapshot"; vars: Record<string, unknown> }
  | { type: "done"; reason: string }
  | { type: "abort"; reason: string }
  | { type: "error"; message: string; where?: string }
  | { type: "close" };

export interface AgentState {
  status: "idle" | "running" | "done" | "aborted" | "error";
  runId: string | null;
  task: string;
  steps: PlanStep[];
  stepStatus: Record<string, StepStatus>;
  currentStepId: string | null;
  planReason: string;
  replanCount: number;
  toolCalls: ToolCallEntry[];
  verdicts: CriticVerdictEntry[];
  latestScreenshot: ScreenshotEntry | null;
  screenshots: ScreenshotEntry[];
  vars: Record<string, unknown>;
  lastError: string | null;
}
