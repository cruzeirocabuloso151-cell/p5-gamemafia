"""
agents/vision.py — VISION, analista visual com Qwen2.5-VL.

Pega as imagens salvas pelo Browser em resources/{run_id}/images/ e usa o
modelo de visão do LM Studio para descrever paleta (hex), estilo, mood,
elementos de UI e como cada referência inspira mecânicas de ML.
"""

from __future__ import annotations

import base64
import mimetypes
from pathlib import Path

from agents.base import BaseAgent


class VisionAgent(BaseAgent):
    name = "vision"

    async def run(self, run_id: str, categoria: str, tema: str) -> str:
        res_dir = Path(self.config.get("resources_dir", "resources")) / run_id / "images"
        images = sorted(res_dir.glob("*")) if res_dir.exists() else []
        max_images = int(self.config.get("max_images_vision", 6))
        images = images[:max_images]

        if not images:
            await self.status("skipped", "Nenhuma imagem coletada pelo Browser")
            return "(sem imagens coletadas — análise visual indisponível)"

        await self.status("running", f"Analisando {len(images)} imagens com Qwen")

        prompt = self.load_prompt("vision.md", categoria=categoria, tema=tema)
        analyses: list[str] = []

        for index, path in enumerate(images, start=1):
            await self.status("running", f"Imagem {index}/{len(images)}: {path.name}")
            try:
                data_uri = self._to_data_uri(path)
                content = [
                    {"type": "text", "text": prompt},
                    {"type": "image_url", "image_url": {"url": data_uri}},
                ]
                description = await self.lm.chat(
                    [{"role": "user", "content": content}],
                    model=self.config["model_vision"],
                    temperature=0.4,
                    max_tokens=600,
                )
                analyses.append(f"### Imagem {index} ({path.name})\n{description}")
                # Emite o resultado parcial para o frontend acompanhar
                await self.emit({
                    "agent": self.name,
                    "status": "streaming",
                    "phase": self.name,
                    "token": f"\n### Imagem {index} ({path.name})\n{description}\n",
                })
            except Exception as exc:  # noqa: BLE001 — uma imagem não derruba o agente
                await self.status("warning", f"Falha na imagem {path.name}: {exc}")

        if not analyses:
            await self.status("skipped", "Nenhuma imagem pôde ser analisada")
            return "(análise visual falhou para todas as imagens)"

        result = "\n\n".join(analyses)
        await self.status("done", f"{len(analyses)} imagens descritas")
        return result

    @staticmethod
    def _to_data_uri(path: Path) -> str:
        """Converte a imagem em data URI base64 para a API de visão."""
        mime = mimetypes.guess_type(path.name)[0] or "image/png"
        encoded = base64.b64encode(path.read_bytes()).decode()
        return f"data:{mime};base64,{encoded}"
