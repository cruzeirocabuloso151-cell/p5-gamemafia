import { useState } from "react";

interface Props {
  onRun: (task: string) => void;
  onCancel: () => void;
  onReset: () => void;
  running: boolean;
}

export function TaskInput({ onRun, onCancel, onReset, running }: Props) {
  const [task, setTask] = useState(
    "abrir https://example.com e extrair o título da página",
  );

  return (
    <div className="task-input">
      <textarea
        value={task}
        onChange={(e) => setTask(e.target.value)}
        placeholder="Descreva a tarefa em português…"
        rows={3}
        disabled={running}
      />
      <div className="row">
        {!running ? (
          <button
            className="primary"
            disabled={!task.trim()}
            onClick={() => onRun(task.trim())}
          >
            Rodar agente
          </button>
        ) : (
          <button className="warn" onClick={onCancel}>
            Cancelar
          </button>
        )}
        <button onClick={onReset} disabled={running}>
          Limpar
        </button>
      </div>
    </div>
  );
}
