"""
orchestrator.py — Pipeline assíncrono dos 6 agentes.

    Maestro → Scout → Browser → Vision → Architect → Judge

- Cada etapa emite eventos numa fila (asyncio.Queue) consumida pelo SSE.
- O estado de cada etapa é persistido no SQLite conforme completa.
- Browser e Vision têm fallback: se falharem, o pipeline continua.
- Cada agente roda sob timeout de segurança (agent_timeout_seconds).
"""

from __future__ import annotations

import asyncio
import traceback

from agents import Architect, BrowserAgent, Judge, Maestro, Scout, VisionAgent
from db import Database
from lm_client import LMClient

# Sentinela que encerra o stream SSE
_END = {"agent": "orchestrator", "status": "end", "phase": "end"}


class Orchestrator:
    def __init__(self, config: dict, db: Database) -> None:
        self.config = config
        self.db = db
        self.lm = LMClient(config["lm_studio_url"])
        # Filas de eventos por run (para o SSE) e tasks em andamento
        self.queues: dict[str, asyncio.Queue] = {}
        self.tasks: dict[str, asyncio.Task] = {}

    # ── API pública ──────────────────────────────────

    async def start_run(self, categoria: str, tema: str) -> str:
        """Cria a run no banco e dispara o pipeline em background."""
        run_id = await self.db.create_run(categoria, tema)
        self.queues[run_id] = asyncio.Queue()
        self.tasks[run_id] = asyncio.create_task(
            self._pipeline(run_id, categoria, tema)
        )
        return run_id

    async def events(self, run_id: str):
        """Gerador assíncrono de eventos SSE para uma run."""
        queue = self.queues.get(run_id)
        if queue is None:
            # Run desconhecida ou já finalizada — devolve o estado e encerra
            run = await self.db.get_run(run_id)
            if run:
                yield {"agent": "orchestrator", "status": run["status"], "phase": run.get("phase")}
            yield _END
            return
        while True:
            event = await queue.get()
            yield event
            if event.get("status") == "end":
                break

    # ── Internals ────────────────────────────────────

    async def _emit(self, run_id: str, event: dict) -> None:
        queue = self.queues.get(run_id)
        if queue is not None:
            await queue.put(event)

    async def _with_timeout(self, coro, agent_name: str):
        """Aplica o timeout de segurança configurável a um agente."""
        timeout = float(self.config.get("agent_timeout_seconds", 180))
        try:
            return await asyncio.wait_for(coro, timeout=timeout)
        except asyncio.TimeoutError:
            raise RuntimeError(f"{agent_name} excedeu o timeout de {timeout:.0f}s")

    async def _pipeline(self, run_id: str, categoria: str, tema: str) -> None:
        async def emit(event: dict) -> None:
            await self._emit(run_id, event)

        def make(agent_cls):
            return agent_cls(self.config, self.lm, emit)

        try:
            await self.db.update_run(run_id, status="running", phase="maestro")

            # ── 1. MAESTRO — briefing criativo ────────────────
            briefing = await self._with_timeout(
                make(Maestro).run(categoria, tema), "Maestro"
            )
            await self.db.update_run(run_id, briefing=briefing, phase="scout")

            # ── 2. SCOUT — queries de busca ───────────────────
            try:
                queries = await self._with_timeout(make(Scout).run(briefing), "Scout")
            except Exception as exc:  # noqa: BLE001 — Scout tem fallback interno,
                # mas se nem isso funcionar seguimos com queries genéricas
                await emit({"agent": "scout", "status": "error", "phase": "scout",
                            "detail": str(exc)})
                queries = ["machine learning game design", "ml visualization games"]
            await self.db.update_run(run_id, queries=queries, phase="browser")

            # ── 3. BROWSER — pesquisa web real (com fallback) ─────
            research: dict | None = None
            try:
                research = await self._with_timeout(
                    make(BrowserAgent).run(queries, run_id), "Browser"
                )
            except Exception as exc:  # noqa: BLE001 — pipeline continua sem pesquisa
                await emit({"agent": "browser", "status": "error", "phase": "browser",
                            "detail": f"Browser falhou (seguindo sem pesquisa): {exc}"})
            await self.db.update_run(run_id, research=research or {}, phase="vision")

            # ── 4. VISION — análise das imagens (com fallback) ────
            vision = ""
            try:
                vision = await self._with_timeout(
                    make(VisionAgent).run(run_id, categoria, tema), "Vision"
                )
            except Exception as exc:  # noqa: BLE001 — pipeline continua sem visão
                await emit({"agent": "vision", "status": "error", "phase": "vision",
                            "detail": f"Vision falhou (seguindo sem análise): {exc}"})
            await self.db.update_run(run_id, vision=vision, phase="architect")

            # ── 5. ARCHITECT — Game Design Document ─────────────
            gdd = await self._with_timeout(
                make(Architect).run(briefing, research, vision, tema), "Architect"
            )
            await self.db.update_run(run_id, gdd=gdd, phase="judge")

            # ── 6. JUDGE — crítica e veredicto ─────────────────
            critique = await self._with_timeout(make(Judge).run(gdd), "Judge")
            await self.db.update_run(
                run_id, critique=critique, status="done", phase="done"
            )
            await emit({"agent": "orchestrator", "status": "done", "phase": "done",
                        "detail": "Pipeline completo"})

        except Exception as exc:  # noqa: BLE001 — falha fatal do pipeline
            traceback.print_exc()
            await self.db.update_run(run_id, status="error", error=str(exc))
            await self._emit(run_id, {
                "agent": "orchestrator", "status": "error",
                "phase": "error", "detail": str(exc),
            })
        finally:
            await self._emit(run_id, _END)
            # Libera a fila depois de um tempo para clientes atrasados conectarem
            asyncio.get_running_loop().call_later(
                60, lambda: self.queues.pop(run_id, None)
            )
            self.tasks.pop(run_id, None)
