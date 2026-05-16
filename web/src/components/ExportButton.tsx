import type { AgentState } from "../types";

interface Props {
  state: AgentState;
}

export function ExportButton({ state }: Props) {
  const hasData = Object.keys(state.vars).length > 0 || state.toolCalls.length > 0;

  const exportJson = () => {
    const blob = makeBlob({
      run_id: state.runId,
      task: state.task,
      status: state.status,
      plan: state.steps,
      vars: state.vars,
      verdicts: state.verdicts,
    }, "application/json");
    download(blob, `agent-run-${state.runId ?? Date.now()}.json`);
  };

  const exportCsv = () => {
    const rows = flattenForCsv(state.vars);
    const csv = toCsv(rows);
    download(makeBlob(csv, "text/csv"), `agent-vars-${state.runId ?? Date.now()}.csv`);
  };

  return (
    <div className="export">
      <strong>Export</strong>
      <div className="row">
        <button onClick={exportJson} disabled={!hasData}>JSON completo</button>
        <button onClick={exportCsv} disabled={Object.keys(state.vars).length === 0}>vars como CSV</button>
      </div>
      {!hasData && <div className="muted">sem dados ainda.</div>}
    </div>
  );
}

function makeBlob(content: unknown, type: string): Blob {
  const text = typeof content === "string" ? content : JSON.stringify(content, null, 2);
  return new Blob([text], { type });
}

function download(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function flattenForCsv(vars: Record<string, unknown>): Record<string, unknown>[] {
  // If any var is an array of objects, treat it as the table; otherwise emit key/value rows.
  for (const [, v] of Object.entries(vars)) {
    if (Array.isArray(v) && v.length > 0 && typeof v[0] === "object") {
      return v as Record<string, unknown>[];
    }
  }
  return Object.entries(vars).map(([k, v]) => ({ key: k, value: stringify(v) }));
}

function stringify(v: unknown): string {
  return typeof v === "string" ? v : JSON.stringify(v);
}

function toCsv(rows: Record<string, unknown>[]): string {
  if (rows.length === 0) return "";
  const cols = Array.from(new Set(rows.flatMap((r) => Object.keys(r))));
  const escape = (v: unknown) => {
    const s = stringify(v ?? "");
    return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const header = cols.join(",");
  const body = rows.map((r) => cols.map((c) => escape(r[c])).join(","));
  return [header, ...body].join("\n");
}
