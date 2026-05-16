import { useEffect } from "react";

interface Handlers {
  onSubmit?: () => void;
  onCancel?: () => void;
  onToggleTheme?: () => void;
}

export function useShortcuts({ onSubmit, onCancel, onToggleTheme }: Handlers) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const meta = e.ctrlKey || e.metaKey;
      if (meta && e.key === "Enter") {
        e.preventDefault();
        onSubmit?.();
      } else if (e.key === "Escape") {
        onCancel?.();
      } else if (meta && e.shiftKey && (e.key === "T" || e.key === "t")) {
        e.preventDefault();
        onToggleTheme?.();
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [onSubmit, onCancel, onToggleTheme]);
}
