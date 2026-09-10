"""
agents/maestro.py — MAESTRO, o orquestrador criativo.

Recebe categoria + tema, pensa profundamente (thinking do Gemma) e produz
o briefing: análise de ML, mecânicas core, conceito inicial e direções
de pesquisa.
"""

from __future__ import annotations

from agents.base import BaseAgent
from catalog import CATEGORIES, THEMES


class Maestro(BaseAgent):
    name = "maestro"

    async def run(self, categoria: str, tema: str) -> str:
        await self.status("running", f"Analisando {categoria} × {tema}")

        prompt = self.load_prompt(
            "maestro.md",
            categoria=categoria,
            categoria_desc=CATEGORIES.get(categoria, categoria),
            tema=tema,
            tema_desc=THEMES.get(tema, tema),
        )
        briefing, _thinking = await self.stream_llm(prompt)

        if not briefing:
            raise RuntimeError("Maestro não produziu briefing")

        await self.status("done", f"Briefing pronto ({len(briefing)} chars)")
        return briefing
