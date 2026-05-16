import { useMemo, useState } from "react";
import { TaskInput } from "./components/TaskInput";
import { PlanBoard } from "./components/PlanBoard";
import { ToolLog } from "./components/ToolLog";
import { CriticVerdict } from "./components/CriticVerdict";
import { BrowserPreview } from "./components/BrowserPreview";
import { ParamControls } from "./components/ParamControls";
import { CredentialsManager } from "./components/CredentialsManager";
import { ExportButton } from "./components/ExportButton";
import { useAgentStream } from "./hooks/useAgentStream";
import { useTheme } from "./hooks/useTheme";
import { useShortcuts } from "./hooks/useShortcuts";

const DEFAULT_BASE = "http://127.0.0.1:8000";

export function App() {
  const [bridgeBase, setBridgeBase] = useState(DEFAULT_BASE);
  const endpoint = useMemo(() => `${bridgeBase}/run`, [bridgeBase]);
  const { state, run, cancel, reset } = useAgentStream(endpoint);
  const running = state.status === "running";
  const { theme, toggle } = useTheme();
  const [task, setTask] = useState(
    "abrir https://example.com e extrair o título da página",
  );

  useShortcuts({
    onSubmit: () => {
      if (!running && task.trim()) run(task.trim());
    },
    onCancel: () => {
      if (running) cancel();
    },
    onToggleTheme: toggle,
  });

  return (
    <div className="app" data-theme={theme}>
      <header>
        <h1>LM Studio Agent <span className="muted">· Gemma 3 + Qwen</span></h1>
        <div className="header-controls">
          <div className="endpoint">
            <label>bridge</label>
            <input
              value={bridgeBase}
              onChange={(e) => setBridgeBase(e.target.value)}
              disabled={running}
              spellCheck={false}
            />
            <span className={`status-dot status-${state.status}`} title={state.status} />
            <span className="status-label">{state.status}</span>
          </div>
          <button className="theme-toggle" onClick={toggle} title="Ctrl+Shift+T">
            {theme === "dark" ? "☀" : "☾"}
          </button>
        </div>
      </header>

      <section className="panel">
        <TaskInput
          task={task}
          onTaskChange={setTask}
          onRun={run}
          onCancel={cancel}
          onReset={reset}
          running={running}
        />
      </section>

      <section className="panel">
        <PlanBoard state={state} />
      </section>

      <section className="grid-3">
        <div className="panel">
          <BrowserPreview latest={state.latestScreenshot} history={state.screenshots} />
        </div>
        <div className="panel">
          <CriticVerdict verdicts={state.verdicts} />
        </div>
        <div className="panel">
          <ToolLog calls={state.toolCalls} />
        </div>
      </section>

      <section className="grid-3">
        <div className="panel">
          <ParamControls bridgeBase={bridgeBase} disabled={running} />
        </div>
        <div className="panel">
          <CredentialsManager bridgeBase={bridgeBase} />
        </div>
        <div className="panel">
          <ExportButton state={state} />
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
