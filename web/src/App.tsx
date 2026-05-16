import { useState } from "react";
import { TaskInput } from "./components/TaskInput";
import { PlanBoard } from "./components/PlanBoard";
import { ToolLog } from "./components/ToolLog";
import { CriticVerdict } from "./components/CriticVerdict";
import { useAgentStream } from "./hooks/useAgentStream";

const DEFAULT_ENDPOINT = "http://127.0.0.1:8000/run";

export function App() {
  const [endpoint, setEndpoint] = useState(DEFAULT_ENDPOINT);
  const { state, run, cancel, reset } = useAgentStream(endpoint);
  const running = state.status === "running";

  return (
    <div className="app">
      <header>
        <h1>LM Studio Agent</h1>
        <div className="endpoint">
          <label>bridge endpoint</label>
          <input
            value={endpoint}
            onChange={(e) => setEndpoint(e.target.value)}
            disabled={running}
            spellCheck={false}
          />
          <span className={`status-dot status-${state.status}`} title={state.status} />
          <span className="status-label">{state.status}</span>
        </div>
      </header>

      <section className="panel">
        <TaskInput onRun={run} onCancel={cancel} onReset={reset} running={running} />
      </section>

      <section className="panel">
        <PlanBoard state={state} />
      </section>

      <section className="grid">
        <div className="panel">
          <CriticVerdict verdicts={state.verdicts} />
        </div>
        <div className="panel">
          <ToolLog calls={state.toolCalls} />
        </div>
      </section>

      {state.lastError && (
        <section className="panel error">
          <strong>Erro</strong>: {state.lastError}
        </section>
      )}
    </div>
  );
}
