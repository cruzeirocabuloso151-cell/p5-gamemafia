import type { CriticVerdictEntry } from "../types";

interface Props {
  verdicts: CriticVerdictEntry[];
}

export function CriticVerdict({ verdicts }: Props) {
  const latest = verdicts[verdicts.length - 1];
  if (!latest) {
    return (
      <div className="critic empty">
        <strong>Crítico</strong>
        <span>nenhum veredicto ainda</span>
      </div>
    );
  }

  const action = latest.next_action;
  const tone = latest.succeeded ? "ok" : action === "abort" ? "abort" : "warn";

  return (
    <div className={`critic critic-${tone}`}>
      <div className="critic-head">
        <strong>Crítico</strong>
        <span className="step-tag">{latest.step_id}</span>
        <span className={`pill pill-${tone}`}>
          {latest.succeeded ? "succeeded" : "failed"} → {action}
        </span>
      </div>
      <div className="critic-reason">{latest.reason}</div>
      {latest.evidence && (
        <div className="critic-evidence">
          <span className="lbl">evidence</span> <code>{latest.evidence}</code>
        </div>
      )}
      {latest.feedback && (
        <div className="critic-feedback">
          <span className="lbl">feedback ao planner</span>
          <div>{latest.feedback}</div>
        </div>
      )}
      {verdicts.length > 1 && (
        <details className="critic-history">
          <summary>histórico ({verdicts.length - 1} anteriores)</summary>
          {verdicts.slice(0, -1).reverse().map((v, i) => (
            <div key={i} className="critic-history-row">
              <span className="step-tag">{v.step_id}</span>
              <span className={`pill pill-${v.succeeded ? "ok" : "warn"}`}>
                {v.succeeded ? "ok" : "fail"} → {v.next_action}
              </span>
              <span className="muted">{v.reason}</span>
            </div>
          ))}
        </details>
      )}
    </div>
  );
}
