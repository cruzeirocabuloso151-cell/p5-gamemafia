"""
db.py — Persistência assíncrona em SQLite (aiosqlite).

Guarda o estado completo de cada run: categoria, tema, saídas de cada
agente, recursos coletados, crítica final e timestamps.
"""

from __future__ import annotations

import json
import uuid
from datetime import datetime, timezone

import aiosqlite

_SCHEMA = """
CREATE TABLE IF NOT EXISTS runs (
    id          TEXT PRIMARY KEY,
    categoria   TEXT NOT NULL,
    tema        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    phase       TEXT,
    briefing    TEXT,
    queries     TEXT,   -- JSON: lista de queries do Scout
    research    TEXT,   -- JSON: sumário da pesquisa do Browser
    vision      TEXT,   -- análise visual do Qwen
    gdd         TEXT,   -- Game Design Document (markdown)
    critique    TEXT,   -- crítica do Judge
    error       TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
"""

# Colunas que aceitam update via update_run()
_UPDATABLE = {
    "status", "phase", "briefing", "queries", "research",
    "vision", "gdd", "critique", "error",
}


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


class Database:
    def __init__(self, path: str) -> None:
        self.path = path

    async def init(self) -> None:
        """Cria as tabelas se não existirem."""
        async with aiosqlite.connect(self.path) as db:
            await db.executescript(_SCHEMA)
            await db.commit()

    async def create_run(self, categoria: str, tema: str) -> str:
        """Cria uma nova run e retorna o run_id."""
        run_id = uuid.uuid4().hex[:12]
        now = _now()
        async with aiosqlite.connect(self.path) as db:
            await db.execute(
                "INSERT INTO runs (id, categoria, tema, status, created_at, updated_at) "
                "VALUES (?, ?, ?, 'pending', ?, ?)",
                (run_id, categoria, tema, now, now),
            )
            await db.commit()
        return run_id

    async def update_run(self, run_id: str, **fields) -> None:
        """Atualiza campos de uma run (serializa dict/list como JSON)."""
        cols, values = [], []
        for key, value in fields.items():
            if key not in _UPDATABLE:
                raise ValueError(f"coluna não atualizável: {key}")
            if isinstance(value, (dict, list)):
                value = json.dumps(value, ensure_ascii=False)
            cols.append(f"{key} = ?")
            values.append(value)
        cols.append("updated_at = ?")
        values.append(_now())
        values.append(run_id)
        async with aiosqlite.connect(self.path) as db:
            await db.execute(
                f"UPDATE runs SET {', '.join(cols)} WHERE id = ?", values
            )
            await db.commit()

    async def get_run(self, run_id: str) -> dict | None:
        async with aiosqlite.connect(self.path) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute("SELECT * FROM runs WHERE id = ?", (run_id,))
            row = await cursor.fetchone()
        if row is None:
            return None
        run = dict(row)
        # Desserializa os campos JSON
        for key in ("queries", "research"):
            if run.get(key):
                try:
                    run[key] = json.loads(run[key])
                except json.JSONDecodeError:
                    pass
        return run

    async def list_runs(self, limit: int = 50) -> list[dict]:
        async with aiosqlite.connect(self.path) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute(
                "SELECT id, categoria, tema, status, phase, error, created_at, updated_at "
                "FROM runs ORDER BY created_at DESC LIMIT ?",
                (limit,),
            )
            rows = await cursor.fetchall()
        return [dict(row) for row in rows]
