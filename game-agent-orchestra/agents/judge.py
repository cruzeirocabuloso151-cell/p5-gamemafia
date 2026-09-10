"""
agents/judge.py — JUDGE, o crítico implacável.

Avalia o GDD com scorecard 0–10 em 5 dimensões, aponta problemas
críticos e dá o veredicto final. Usa Gemma com thinking.
"""

from __future__ import annotations

from agents.base import BaseAgent


class Judge(BaseAgent):
    name = "judge"

    async def run(self, gdd: str) -> str:
        await self.status("running", "Julgando o Game Design Document")

        prompt = self.load_prompt("judge.md", gdd=gdd)
        critique, _thinking = await self.stream_llm(prompt, temperature=0.5)

        if not critique:
            raise RuntimeError("Judge não produziu a crítica")

        await self.status("done", "Veredicto emitido")
        return critique
