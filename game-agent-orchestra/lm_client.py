"""
lm_client.py — Cliente streaming para o LM Studio (API OpenAI-compatible).

Responsabilidades:
  1. Fazer requisições de chat com stream=True e parsear o SSE linha a linha.
  2. Extrair tanto `choices[0].delta.content` quanto `choices[0].delta.reasoning_content`.
  3. Separar "thinking" (raciocínio) do output final, incluindo modelos que
     emitem o raciocínio dentro de tags <think>...</think> no próprio content.
"""

from __future__ import annotations

import json
import re
from typing import AsyncGenerator

import httpx

# Tags de raciocínio reconhecidas (algumas variantes comuns)
_THINK_OPEN = re.compile(r"<think(?:ing)?>", re.IGNORECASE)
_THINK_CLOSE = re.compile(r"</think(?:ing)?>", re.IGNORECASE)

# Maior tag possível — usado para segurar um sufixo que pode ser uma tag parcial
_MAX_TAG_LEN = len("</thinking>")


class ThinkingParser:
    """
    Parser incremental de tags <think>/<thinking>.

    Recebe chunks arbitrários de texto (que podem cortar uma tag ao meio)
    e devolve pares (tipo, texto) onde tipo é "thinking" ou "token".
    Funciona mesmo com a tag ainda aberta (thinking sendo gerado ao vivo).
    """

    def __init__(self) -> None:
        self._buffer = ""
        self._in_think = False

    def _possible_tag_suffix(self, text: str) -> int:
        """
        Retorna quantos caracteres do fim de `text` podem ser o começo
        de uma tag (ex.: o chunk terminou em "</thi"). Esses caracteres
        ficam retidos no buffer até o próximo chunk.
        """
        max_check = min(len(text), _MAX_TAG_LEN - 1)
        for size in range(max_check, 0, -1):
            tail = text[-size:]
            # candidatos: prefixos de "<think>", "<thinking>", "</think>", "</thinking>"
            for tag in ("<think>", "<thinking>", "</think>", "</thinking>"):
                if tag.startswith(tail.lower()):
                    return size
        return 0

    def feed(self, chunk: str) -> list[tuple[str, str]]:
        """Processa um chunk e retorna a lista de eventos extraídos."""
        self._buffer += chunk
        events: list[tuple[str, str]] = []

        while self._buffer:
            if self._in_think:
                match = _THINK_CLOSE.search(self._buffer)
                if match:
                    # Tudo antes do </think> é thinking; continua após a tag
                    if match.start() > 0:
                        events.append(("thinking", self._buffer[: match.start()]))
                    self._buffer = self._buffer[match.end():]
                    self._in_think = False
                else:
                    # Tag ainda aberta: emite o que der, retendo possível tag parcial
                    hold = self._possible_tag_suffix(self._buffer)
                    emit = self._buffer[: len(self._buffer) - hold]
                    if emit:
                        events.append(("thinking", emit))
                    self._buffer = self._buffer[len(self._buffer) - hold:]
                    break
            else:
                match = _THINK_OPEN.search(self._buffer)
                if match:
                    if match.start() > 0:
                        events.append(("token", self._buffer[: match.start()]))
                    self._buffer = self._buffer[match.end():]
                    self._in_think = True
                else:
                    hold = self._possible_tag_suffix(self._buffer)
                    emit = self._buffer[: len(self._buffer) - hold]
                    if emit:
                        events.append(("token", emit))
                    self._buffer = self._buffer[len(self._buffer) - hold:]
                    break
        return events

    def flush(self) -> list[tuple[str, str]]:
        """Esvazia o buffer no fim do stream (tag parcial vira texto normal)."""
        events: list[tuple[str, str]] = []
        if self._buffer:
            kind = "thinking" if self._in_think else "token"
            events.append((kind, self._buffer))
            self._buffer = ""
        return events


class LMClient:
    """Cliente assíncrono para o endpoint /chat/completions do LM Studio."""

    def __init__(self, base_url: str, timeout: float = 300.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout

    async def stream_chat(
        self,
        messages: list[dict],
        model: str,
        temperature: float = 0.75,
        max_tokens: int = 4096,
    ) -> AsyncGenerator[tuple[str, str], None]:
        """
        Faz streaming da resposta. Gera pares (tipo, texto):
          - ("thinking", txt) → raciocínio (reasoning_content OU tags <think>)
          - ("token", txt)    → output final
        """
        payload = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "max_tokens": max_tokens,
            "stream": True,
        }
        parser = ThinkingParser()

        async with httpx.AsyncClient(timeout=self.timeout) as client:
            async with client.stream(
                "POST", f"{self.base_url}/chat/completions", json=payload
            ) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    line = line.strip()
                    if not line.startswith("data:"):
                        continue
                    data = line[len("data:"):].strip()
                    if data == "[DONE]":
                        break
                    try:
                        obj = json.loads(data)
                    except json.JSONDecodeError:
                        continue  # linha SSE malformada — ignora
                    choices = obj.get("choices") or []
                    if not choices:
                        continue
                    delta = choices[0].get("delta") or {}

                    # Campo dedicado de raciocínio (LM Studio / modelos "thinking")
                    reasoning = delta.get("reasoning_content")
                    if reasoning:
                        yield ("thinking", reasoning)

                    # Conteúdo normal — pode conter tags <think> embutidas
                    content = delta.get("content")
                    if content:
                        for event in parser.feed(content):
                            yield event

        for event in parser.flush():
            yield event

    async def chat(
        self,
        messages: list[dict],
        model: str,
        temperature: float = 0.4,
        max_tokens: int = 2048,
    ) -> str:
        """Chamada não-streaming (usada pelo agente de visão)."""
        payload = {
            "model": model,
            "messages": messages,
            "temperature": temperature,
            "max_tokens": max_tokens,
            "stream": False,
        }
        async with httpx.AsyncClient(timeout=self.timeout) as client:
            response = await client.post(
                f"{self.base_url}/chat/completions", json=payload
            )
            response.raise_for_status()
            obj = response.json()
            content = obj["choices"][0]["message"]["content"] or ""
            # Remove blocos <think> completos de respostas não-streaming
            content = re.sub(
                r"<think(?:ing)?>.*?</think(?:ing)?>",
                "",
                content,
                flags=re.DOTALL | re.IGNORECASE,
            )
            return content.strip()
