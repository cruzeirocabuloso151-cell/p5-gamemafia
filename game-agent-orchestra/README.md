# 🎮 Game Agent Orchestra

Sistema **multi-agente 100% local** que orquestra 6 agentes de IA para projetar games onde **Machine Learning é o protagonista visível** — não um backend escondido. Conecta ao **LM Studio** e faz pesquisa web real com Playwright.

```
Maestro → Scout → Browser (Playwright) → Vision (Qwen) → Architect → Judge
```

| Agente | Papel | Modelo |
|---|---|---|
| 🎼 **Maestro** | Analisa categoria×tema, produz briefing (thinking + streaming) | gemma-3-12b-it |
| 🔍 **Scout** | Extrai 7 queries de busca em JSON estrito | gemma-3-12b-it |
| 🌐 **Browser** | Navega de verdade (Playwright), salva HTML/screenshots/imagens em `resources/` | — |
| 👁 **Vision** | Descreve as imagens coletadas: paleta hex, estilo, mood, UI | qwen2.5-vl-7b-instruct |
| 📐 **Architect** | Escreve o Game Design Document completo (markdown) | gemma-3-12b-it |
| ⚖️ **Judge** | Scorecard 0–10 em 5 dimensões + veredicto implacável | gemma-3-12b-it |

Tudo é transmitido ao frontend por **SSE** token a token, com o *thinking* do Gemma separado do output final. Estado persistido em **SQLite**; recursos por run em `resources/{run_id}/`.

## 1. Pré-requisitos

- **Python 3.11+** e **Node 18+**
- **[LM Studio](https://lmstudio.ai/)** rodando o servidor local na porta **2234**

### Carregando os 2 modelos no LM Studio

1. Abra o LM Studio → aba **Discover** e baixe:
   - `gemma-3-12b-it` (modelo principal, texto)
   - `qwen2.5-vl-7b-instruct` (modelo de visão)
2. Aba **Developer** → **Start Server**.
3. Em *Server Port*, configure **2234** (ou ajuste `lm_studio_url` no `config.yaml`).
4. Carregue **os dois modelos** (o LM Studio atende múltiplos modelos pelo mesmo endpoint; a API escolhe pelo campo `model` da requisição).
5. Confira: `curl http://localhost:2234/v1/models` deve listar ambos.

## 2. Instalação

```bash
cd game-agent-orchestra

# Backend
python -m venv .venv && source .venv/bin/activate   # Windows: .venv\Scripts\activate
pip install -r requirements.txt
playwright install chromium          # navegador headless do agente Browser

# Frontend (build único, servido pelo backend)
cd frontend
npm install
npm run build
cd ..
```

> Opcional: exporte `SERPAPI_KEY` para usar SerpAPI na busca; sem a key, o Browser usa DuckDuckGo HTML.

## 3. Rodando

Um único comando após o setup:

```bash
uvicorn main:app --port 8000
```

Abra **http://localhost:8000** — selecione a **categoria de ML** e o **tema visual**, clique em *Iniciar Pipeline* e acompanhe:

- painel dos 6 agentes com status ao vivo;
- coluna de **thinking** do Gemma streamando em tempo real;
- aba **Game Design** com o GDD renderizado + crítica do Judge;
- aba **Recursos** com imagens, screenshots e links salvos.

### Modo dev do frontend (hot reload)

```bash
cd frontend && npm run dev   # http://localhost:5173, proxia /api para :8000
```

## 4. API

| Endpoint | Descrição |
|---|---|
| `POST /api/run` `{categoria, tema}` | inicia o pipeline → `{run_id}` |
| `GET /api/run/stream?run_id=` | **SSE**: `{agent, status, token, thinking, phase}` |
| `GET /api/run/{run_id}` | estado completo da run |
| `GET /api/runs` | histórico |
| `GET /api/resources/{run_id}` | recursos salvos (imagens, links, sumários) |
| `GET /api/catalog` | categorias e temas disponíveis |

## 5. Configuração (`config.yaml`)

```yaml
lm_studio_url: http://localhost:2234/v1
model_main: gemma-3-12b-it
model_vision: qwen2.5-vl-7b-instruct
temperature: 0.75
max_tokens: 4096
browser_headless: true
max_pages_per_query: 3
agent_timeout_seconds: 180
```

## 6. Tolerância a falhas

- **Browser** ou **Vision** falharam? O pipeline **continua** com fallback (o GDD é escrito só com o briefing) — o erro é logado no stream e no banco.
- Cada agente roda sob **timeout** configurável (`agent_timeout_seconds`).
- Playwright indisponível? O Browser cai para `httpx` + BeautifulSoup automaticamente.
- `robots.txt` é respeitado (checagem básica por host, com cache).

## 7. Estrutura

```
game-agent-orchestra/
├── config.yaml           # configuração central
├── main.py               # FastAPI + SSE + estáticos
├── orchestrator.py       # pipeline assíncrono dos 6 agentes
├── lm_client.py          # cliente streaming LM Studio (+ parser <think>)
├── db.py                 # SQLite async (aiosqlite)
├── catalog.py            # categorias de ML e temas visuais
├── agents/               # os 6 agentes
├── prompts/              # templates de prompt de cada agente
├── resources/            # HTML, screenshots e imagens por run
└── frontend/             # React + Vite
```
