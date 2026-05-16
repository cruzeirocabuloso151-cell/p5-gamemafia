import { useEffect, useState } from "react";

interface Props {
  task: string;
  onTaskChange: (s: string) => void;
  onRun: (task: string) => void;
  onCancel: () => void;
  onReset: () => void;
  running: boolean;
}

export function TaskInput({ task, onTaskChange, onRun, onCancel, onReset, running }: Props) {
  const [local, setLocal] = useState(task);
  useEffect(() => setLocal(task), [task]);

  const commit = () => {
    onTaskChange(local);
    onRun(local.trim());
  };

  return (
    <div className="task-input">
      <textarea
        value={local}
        onChange={(e) => setLocal(e.target.value)}
        placeholder="Descreva a tarefa em português… (Ctrl+Enter para rodar)"
        rows={3}
        disabled={running}
      />
      <div className="row">
        {!running ? (
          <button className="primary" disabled={!local.trim()} onClick={commit}>
            Rodar agente <kbd>Ctrl+Enter</kbd>
          </button>
        ) : (
          <button className="warn" onClick={onCancel}>
            Cancelar <kbd>Esc</kbd>
          </button>
        )}
        <button onClick={onReset} disabled={running}>Limpar</button>
      </div>
    </div>
  );
}
