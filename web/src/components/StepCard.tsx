import type { PlanStep, StepStatus } from "../types";

const LABEL: Record<StepStatus, string> = {
  pending: "pendente",
  running: "rodando",
  succeeded: "sucesso",
  failed: "falhou",
  replanned: "replan",
};

interface Props {
  step: PlanStep;
  status: StepStatus;
  isCurrent: boolean;
}

export function StepCard({ step, status, isCurrent }: Props) {
  return (
    <div className={`step-card status-${status} ${isCurrent ? "current" : ""}`}>
      <div className="step-head">
        <span className="step-id">{step.id}</span>
        <span className={`status-pill status-${status}`}>{LABEL[status]}</span>
      </div>
      <div className="step-goal">{step.goal}</div>
      {step.success_criteria && (
        <div className="step-criteria" title="success_criteria">
          ✓ {step.success_criteria}
        </div>
      )}
    </div>
  );
}
