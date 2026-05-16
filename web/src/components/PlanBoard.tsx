import type { AgentState } from "../types";
import { StepCard } from "./StepCard";

interface Props {
  state: AgentState;
}

export function PlanBoard({ state }: Props) {
  if (state.steps.length === 0) {
    return (
      <div className="plan-board empty">
        <span>Sem plano ainda. Rode uma tarefa para o planejador gerar steps.</span>
      </div>
    );
  }
  return (
    <div className="plan-board">
      <div className="plan-header">
        <strong>Plano</strong>
        {state.replanCount > 0 && (
          <span className="replan-badge">replan #{state.replanCount}</span>
        )}
        <span className="plan-reason">{state.planReason}</span>
      </div>
      <div className="steps-row">
        {state.steps.map((s) => (
          <StepCard
            key={s.id}
            step={s}
            status={state.stepStatus[s.id] ?? "pending"}
            isCurrent={state.currentStepId === s.id}
          />
        ))}
      </div>
    </div>
  );
}
