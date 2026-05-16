import { useEffect, useRef } from "react";
import type { ToolCallEntry } from "../types";

interface Props {
  calls: ToolCallEntry[];
}

function fmt(value: unknown): string {
  try {
    const s = JSON.stringify(value, null, 0);
    return s.length > 240 ? s.slice(0, 240) + "…" : s;
  } catch {
    return String(value);
  }
}

export function ToolLog({ calls }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight, behavior: "smooth" });
  }, [calls.length]);

  return (
    <div className="tool-log" ref={ref}>
      <div className="log-header">
        <strong>Tool calls</strong>
        <span className="count">{calls.length}</span>
      </div>
      {calls.length === 0 && <div className="empty">aguardando ações…</div>}
      {calls.map((c, i) => (
        <div className="log-row" key={i}>
          <div className="log-meta">
            <span className="tag step">{c.step_id}</span>
            <span className="tag iter">it{c.iter}</span>
            <span className="tag tool">{c.tool}</span>
            <span className="tag id">{c.id}</span>
          </div>
          <div className="log-params">
            <span className="lbl">params</span> <code>{fmt(c.params)}</code>
          </div>
          {c.executor_output && (
            <div className="log-exec">
              <span className="lbl">gemma</span>{" "}
              <code>
                {c.executor_output.length > 240
                  ? c.executor_output.slice(0, 240) + "…"
                  : c.executor_output}
              </code>
            </div>
          )}
          <div className="log-result">
            <span className="lbl">result</span> <code>{fmt(c.result)}</code>
          </div>
        </div>
      ))}
    </div>
  );
}
