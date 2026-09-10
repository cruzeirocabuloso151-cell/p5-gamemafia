"""
agents/architect.py — ARCHITECT, autor do Game Design Document.

Recebe briefing + pesquisa web + análise visual e escreve o GDD completo
em markdown, com streaming + thinking do Gemma.
"""

from __future__ import annotations

import json

from agents.base import BaseAgent


class Architect(BaseAgent):
    name = "architect"

    async def run(
        self, briefing: str, research: dict | None, vision: str, tema: str
    ) -> str:
        await self.status("running", "Escrevendo o Game Design Document")

        prompt = self.load_prompt(
            "architect.md",
            briefing=briefing,
            research=self._summarize_research(research),
            vision=vision or "(sem análise visual)",
            tema=tema,
        )
        gdd, _thinking = await self.stream_llm(prompt)

        if not gdd:
            raise RuntimeError("Architect não produziu o GDD")

        await self.status("done", f"GDD pronto ({len(gdd)} chars)")
        return gdd

    @staticmethod
    def _summarize_research(research: dict | None) -> str:
        """Compacta a pesquisa do Browser num texto que cabe no contexto."""
        if not research or not research.get("pages"):
            return "(pesquisa web indisponível — projete a partir do briefing)"
        parts: list[str] = []
        for page in research["pages"][:10]:
            parts.append(
                f"- **{page.get('title', page['url'])}** ({page['url']})\n"
                f"  {page.get('text_excerpt', '')[:800]}"
            )
        queries = [q["query"] for q in research.get("queries", [])]
        header = f"Queries pesquisadas: {json.dumps(queries, ensure_ascii=False)}\n\n"
        return header + "\n".join(parts)
