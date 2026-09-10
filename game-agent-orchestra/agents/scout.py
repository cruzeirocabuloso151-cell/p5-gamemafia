"""
agents/scout.py — SCOUT, extrator de queries de busca.

Lê o briefing do Maestro e devolve 7 queries otimizadas em JSON estrito.
Tem parser tolerante: extrai o JSON mesmo se o modelo enfeitar a resposta.
"""

from __future__ import annotations

import json
import re

from agents.base import BaseAgent


class Scout(BaseAgent):
    name = "scout"

    async def run(self, briefing: str) -> list[str]:
        await self.status("running", "Extraindo queries de busca")

        prompt = self.load_prompt("scout.md", briefing=briefing)
        raw, _thinking = await self.stream_llm(
            prompt, temperature=0.3, max_tokens=1024
        )

        queries = self._parse_queries(raw)
        if not queries:
            # Fallback: gera queries genéricas a partir do briefing
            queries = self._fallback_queries(briefing)

        queries = queries[:7]
        await self.status("done", f"{len(queries)} queries prontas")
        return queries

    @staticmethod
    def _parse_queries(raw: str) -> list[str]:
        """Extrai {\"queries\": [...]} mesmo com texto extra ao redor."""
        # Remove cercas de código se houver
        raw = re.sub(r"```(?:json)?", "", raw).strip("` \n")
        # Tenta o JSON completo primeiro, depois procura o primeiro objeto
        candidates = [raw]
        match = re.search(r"\{.*\}", raw, re.DOTALL)
        if match:
            candidates.append(match.group(0))
        for candidate in candidates:
            try:
                obj = json.loads(candidate)
                queries = obj.get("queries", [])
                if isinstance(queries, list):
                    return [str(q).strip() for q in queries if str(q).strip()]
            except (json.JSONDecodeError, AttributeError):
                continue
        return []

    @staticmethod
    def _fallback_queries(briefing: str) -> list[str]:
        """Queries de emergência se o JSON do modelo falhar."""
        first_line = briefing.strip().splitlines()[0][:60] if briefing.strip() else "ml game"
        return [
            "machine learning visible gameplay games",
            "educational machine learning game design",
            "neural network visualization interactive",
            f"{first_line} game concept art",
            "games that teach AI concepts",
        ]
