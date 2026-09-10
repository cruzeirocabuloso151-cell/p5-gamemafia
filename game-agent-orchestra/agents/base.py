"""
agents/base.py — Classe base de todos os agentes.

Fornece:
  - carregamento de templates de prompt da pasta prompts/
  - streaming do LLM com separação thinking/output e emissão de eventos SSE
  - helpers de status
"""

from __future__ import annotations

from pathlib import Path
from typing import Awaitable, Callable

from lm_client import LMClient

PROMPTS_DIR = Path(__file__).resolve().parent.parent / "prompts"

# Assinatura do callback de eventos: recebe um dict e publica no stream SSE
EmitFn = Callable[[dict], Awaitable[None]]


class BaseAgent:
    """Classe base: cada agente concreto define `name` e implementa `run()`."""

    name: str = "base"

    def __init__(self, config: dict, lm: LMClient, emit: EmitFn) -> None:
        self.config = config
        self.lm = lm
        self.emit = emit

    # ── Prompts ────────────────────────────────────

    def load_prompt(self, filename: str, **kwargs) -> str:
        """Lê um template de prompts/ e interpola as variáveis."""
        template = (PROMPTS_DIR / filename).read_text(encoding="utf-8")
        return template.format(**kwargs)

    # ── Eventos SSE ────────────────────────────────

    async def status(self, status: str, detail: str = "") -> None:
        """Emite um evento de mudança de status do agente."""
        await self.emit({
            "agent": self.name,
            "status": status,
            "phase": self.name,
            "detail": detail,
        })

    # ── LLM streaming ──────────────────────────────

    async def stream_llm(
        self,
        prompt: str,
        model: str | None = None,
        system: str | None = None,
        temperature: float | None = None,
        max_tokens: int | None = None,
    ) -> tuple[str, str]:
        """
        Chama o LLM em streaming, emitindo cada pedaço como evento SSE
        ({token} ou {thinking}). Retorna (output_final, thinking_completo).
        """
        messages: list[dict] = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        output_parts: list[str] = []
        thinking_parts: list[str] = []

        async for kind, text in self.lm.stream_chat(
            messages,
            model=model or self.config["model_main"],
            temperature=temperature if temperature is not None else self.config["temperature"],
            max_tokens=max_tokens or self.config["max_tokens"],
        ):
            if kind == "thinking":
                thinking_parts.append(text)
                await self.emit({
                    "agent": self.name,
                    "status": "streaming",
                    "phase": self.name,
                    "thinking": text,
                })
            else:
                output_parts.append(text)
                await self.emit({
                    "agent": self.name,
                    "status": "streaming",
                    "phase": self.name,
                    "token": text,
                })

        return "".join(output_parts).strip(), "".join(thinking_parts).strip()
