import { useState } from "react";
import type { ScreenshotEntry } from "../types";

interface Props {
  latest: ScreenshotEntry | null;
  history: ScreenshotEntry[];
}

export function BrowserPreview({ latest, history }: Props) {
  const [pinned, setPinned] = useState<ScreenshotEntry | null>(null);
  const shown = pinned ?? latest;

  if (!shown) {
    return (
      <div className="preview empty">
        <strong>Browser preview</strong>
        <span>nenhum screenshot ainda (precisa do Playwright instalado)</span>
      </div>
    );
  }

  return (
    <div className="preview">
      <div className="preview-head">
        <strong>Browser preview</strong>
        <span className="preview-meta">
          {shown.url && <code title={shown.url}>{truncateUrl(shown.url)}</code>}
          <span className="muted">após {shown.after_tool}</span>
        </span>
        {pinned && (
          <button className="link" onClick={() => setPinned(null)}>
            voltar ao mais recente
          </button>
        )}
      </div>
      <img className="preview-img" src={shown.data_url} alt="browser preview" />
      {history.length > 1 && (
        <div className="preview-history">
          {history.slice(-12).reverse().map((s, i) => (
            <button
              key={i}
              className={`thumb ${s === shown ? "active" : ""}`}
              onClick={() => setPinned(s)}
              title={`${s.after_tool} · ${s.url ?? ""}`}
            >
              <img src={s.data_url} alt="" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

function truncateUrl(url: string): string {
  return url.length > 60 ? url.slice(0, 57) + "…" : url;
}
