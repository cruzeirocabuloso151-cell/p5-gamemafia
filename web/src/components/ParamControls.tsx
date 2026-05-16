import { useEffect, useState } from "react";
import type { RuntimeConfigDTO } from "../types";

interface Props {
  bridgeBase: string;
  disabled: boolean;
}

export function ParamControls({ bridgeBase, disabled }: Props) {
  const [cfg, setCfg] = useState<RuntimeConfigDTO | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`${bridgeBase}/config`)
      .then((r) => r.json())
      .then((data: RuntimeConfigDTO) => {
        if (!cancelled) setCfg(data);
      })
      .catch((e) => !cancelled && setError(`não consegui ler /config: ${e.message}`));
    return () => {
      cancelled = true;
    };
  }, [bridgeBase]);

  if (error) return <div className="param-controls error">{error}</div>;
  if (!cfg) return <div className="param-controls"><strong>Parâmetros</strong><span className="muted">carregando…</span></div>;

  const patch = async (delta: Partial<RuntimeConfigDTO>) => {
    setPending(true);
    setError(null);
    try {
      const resp = await fetch(`${bridgeBase}/config`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(delta),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const next = (await resp.json()) as RuntimeConfigDTO;
      setCfg(next);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="param-controls">
      <div className="pc-head">
        <strong>Parâmetros</strong>
        {pending && <span className="muted">salvando…</span>}
      </div>
      <NumberField label="max_step_iters" value={cfg.max_step_iters} min={1} max={32}
                   onChange={(v) => patch({ max_step_iters: v })} disabled={disabled || pending} />
      <NumberField label="max_replans" value={cfg.max_replans} min={0} max={10}
                   onChange={(v) => patch({ max_replans: v })} disabled={disabled || pending} />
      <RangeField label="router_temperature" value={cfg.router_temperature} min={0} max={1} step={0.05}
                  onChange={(v) => patch({ router_temperature: v })} disabled={disabled || pending} />
      <RangeField label="executor_temperature" value={cfg.executor_temperature} min={0} max={1.5} step={0.05}
                  onChange={(v) => patch({ executor_temperature: v })} disabled={disabled || pending} />
      <NumberField label="screenshot_max_width" value={cfg.screenshot_max_width} min={320} max={2048} step={64}
                   onChange={(v) => patch({ screenshot_max_width: v })} disabled={disabled || pending} />
    </div>
  );
}

function NumberField({ label, value, min, max, step, onChange, disabled }: {
  label: string; value: number; min: number; max: number; step?: number;
  onChange: (v: number) => void; disabled: boolean;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      <input type="number" value={value} min={min} max={max} step={step ?? 1}
             disabled={disabled}
             onChange={(e) => onChange(Number(e.target.value))} />
    </label>
  );
}

function RangeField({ label, value, min, max, step, onChange, disabled }: {
  label: string; value: number; min: number; max: number; step: number;
  onChange: (v: number) => void; disabled: boolean;
}) {
  return (
    <label className="field range">
      <span>{label} <em>{value.toFixed(2)}</em></span>
      <input type="range" value={value} min={min} max={max} step={step}
             disabled={disabled}
             onChange={(e) => onChange(Number(e.target.value))} />
    </label>
  );
}
