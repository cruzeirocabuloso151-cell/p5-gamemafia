import { useEffect, useState } from "react";

interface Props {
  bridgeBase: string;
}

export function CredentialsManager({ bridgeBase }: Props) {
  const [known, setKnown] = useState<string[]>([]);
  const [name, setName] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState<string | null>(null);

  const refresh = async () => {
    try {
      const r = await fetch(`${bridgeBase}/credentials`);
      const d = await r.json();
      setKnown(d.known ?? []);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  useEffect(() => {
    refresh();
  }, [bridgeBase]);

  const add = async () => {
    if (!name.trim()) return;
    setError(null);
    try {
      const r = await fetch(`${bridgeBase}/credentials`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ [name.trim()]: secret }),
      });
      const d = await r.json();
      setKnown(d.known ?? []);
      setName("");
      setSecret("");
    } catch (e) {
      setError((e as Error).message);
    }
  };

  const remove = async (n: string) => {
    setError(null);
    try {
      const r = await fetch(`${bridgeBase}/credentials/${encodeURIComponent(n)}`, {
        method: "DELETE",
      });
      const d = await r.json();
      setKnown(d.known ?? []);
    } catch (e) {
      setError((e as Error).message);
    }
  };

  return (
    <div className="creds">
      <strong>Credenciais</strong>
      <div className="muted creds-hint">
        referenciadas como <code>&lt;credencial:nome&gt;</code> em prompts e params. valores nunca aparecem nos logs.
      </div>
      {known.length === 0 && <div className="muted">nenhuma cadastrada.</div>}
      {known.length > 0 && (
        <ul className="creds-list">
          {known.map((n) => (
            <li key={n}>
              <code>{n}</code>
              <button className="link danger" onClick={() => remove(n)}>remover</button>
            </li>
          ))}
        </ul>
      )}
      <div className="creds-add">
        <input placeholder="nome (ex: prod_user)" value={name} onChange={(e) => setName(e.target.value)} />
        <input
          placeholder="valor (secret)"
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
        <button onClick={add} disabled={!name.trim()}>adicionar</button>
      </div>
      {error && <div className="error">{error}</div>}
    </div>
  );
}
