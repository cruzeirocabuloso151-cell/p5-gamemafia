"""
main.py — FastAPI app do game-agent-orchestra.

Endpoints:
  POST /api/run                  → inicia o pipeline, retorna run_id
  GET  /api/run/stream?run_id=   → SSE com eventos {agent, status, token, thinking, phase}
  GET  /api/run/{run_id}         → estado completo da run
  GET  /api/runs                 → histórico de runs
  GET  /api/resources/{run_id}   → recursos salvos (imagens, links, sumários)
  GET  /api/catalog              → categorias de ML e temas visuais

Também serve o frontend buildado (frontend/dist) e os arquivos de resources/.

Rodar: uvicorn main:app --port 8000
"""

from __future__ import annotations

import json
from contextlib import asynccontextmanager
from pathlib import Path

import yaml
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, StreamingResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel

from catalog import CATEGORIES, THEMES
from db import Database
from orchestrator import Orchestrator

BASE_DIR = Path(__file__).resolve().parent

# ── Configuração central ───────────────────────────────
with open(BASE_DIR / "config.yaml", encoding="utf-8") as fh:
    CONFIG: dict = yaml.safe_load(fh)

# Caminhos relativos ao diretório do projeto
CONFIG["resources_dir"] = str(BASE_DIR / CONFIG.get("resources_dir", "resources"))
DB_PATH = str(BASE_DIR / CONFIG.get("database_path", "orchestra.db"))

db = Database(DB_PATH)
orchestrator = Orchestrator(CONFIG, db)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Inicializa o banco na subida do servidor."""
    await db.init()
    Path(CONFIG["resources_dir"]).mkdir(parents=True, exist_ok=True)
    yield


app = FastAPI(title="game-agent-orchestra", lifespan=lifespan)

# CORS liberado para o dev server do Vite (localhost:5173)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


class RunRequest(BaseModel):
    categoria: str
    tema: str


# ══════════════════ API ══════════════════

@app.get("/api/catalog")
async def get_catalog():
    """Categorias de ML e temas visuais disponíveis."""
    return {"categorias": CATEGORIES, "temas": THEMES}


@app.post("/api/run")
async def create_run(body: RunRequest):
    """Inicia o pipeline de agentes para categoria + tema."""
    if body.categoria not in CATEGORIES:
        raise HTTPException(400, f"categoria inválida: {body.categoria}")
    if body.tema not in THEMES:
        raise HTTPException(400, f"tema inválido: {body.tema}")
    run_id = await orchestrator.start_run(body.categoria, body.tema)
    return {"run_id": run_id}


@app.get("/api/run/stream")
async def stream_run(run_id: str):
    """SSE: eventos ao vivo do pipeline (status, tokens, thinking)."""

    async def generator():
        async for event in orchestrator.events(run_id):
            yield f"data: {json.dumps(event, ensure_ascii=False)}\n\n"

    return StreamingResponse(
        generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",  # desliga buffering em proxies
        },
    )


@app.get("/api/runs")
async def list_runs():
    """Histórico de runs (mais recentes primeiro)."""
    return await db.list_runs()


@app.get("/api/run/{run_id}")
async def get_run(run_id: str):
    """Estado completo de uma run."""
    run = await db.get_run(run_id)
    if run is None:
        raise HTTPException(404, "run não encontrada")
    return run


@app.get("/api/resources/{run_id}")
async def get_resources(run_id: str):
    """Recursos salvos pelo Browser: imagens, screenshots, links, metadados."""
    res_dir = Path(CONFIG["resources_dir"]) / run_id
    if not res_dir.exists():
        return {"run_id": run_id, "images": [], "screenshots": [], "pages": [], "queries": []}

    metadata: dict = {}
    meta_file = res_dir / "metadata.json"
    if meta_file.exists():
        metadata = json.loads(meta_file.read_text(encoding="utf-8"))

    def rel_urls(subdir: str) -> list[str]:
        folder = res_dir / subdir
        if not folder.exists():
            return []
        return [
            f"/resources/{run_id}/{subdir}/{f.name}"
            for f in sorted(folder.iterdir()) if f.is_file()
        ]

    return {
        "run_id": run_id,
        "images": rel_urls("images"),
        "screenshots": rel_urls("screenshots"),
        "pages": [
            {"url": p["url"], "title": p.get("title", ""), "query": p.get("query", "")}
            for p in metadata.get("pages", [])
        ],
        "queries": metadata.get("queries", []),
        "downloaded": metadata.get("images", []),
    }


# ══════════════════ Estáticos ══════════════════

# Recursos coletados (imagens, screenshots) servidos direto
app.mount(
    "/resources",
    StaticFiles(directory=CONFIG["resources_dir"], check_dir=False),
    name="resources",
)

# Frontend buildado pelo Vite (frontend/dist) — se existir
FRONTEND_DIST = BASE_DIR / "frontend" / "dist"
if FRONTEND_DIST.exists():
    app.mount(
        "/assets", StaticFiles(directory=FRONTEND_DIST / "assets"), name="assets"
    )

    @app.get("/")
    async def index():
        return FileResponse(FRONTEND_DIST / "index.html")
else:

    @app.get("/")
    async def index_placeholder():
        return {
            "app": "game-agent-orchestra",
            "hint": "frontend não buildado — rode `npm install && npm run build` em frontend/, "
                    "ou use o dev server (`npm run dev`) em http://localhost:5173",
        }
