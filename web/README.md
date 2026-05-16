# LM Studio Agent — Web Client

UI React/Vite/TS para o agente Plan-and-Execute do `lmstudio_prompts/bridge.py`.

## Como rodar

1. Suba o LM Studio com os dois modelos (ver `lmstudio_prompts/lmstudio_integration_guide.md`).
2. Suba o bridge em modo servidor:
   ```bash
   python3 ../lmstudio_prompts/bridge.py --serve 8000
   ```
3. Em outro terminal:
   ```bash
   cd web
   npm install
   npm run dev
   ```
4. Abra `http://localhost:5173`.

## O que aparece na UI

- **Endpoint**: por padrão `http://127.0.0.1:8000/run` (editável no canto superior direito).
- **Task input**: textarea para descrever a tarefa em PT, botões `Rodar` / `Cancelar` / `Limpar`.
- **Plan board**: cada step do planejador vira um card colorido por status (pendente / rodando / sucesso / falhou / replan). Mostra `goal` e `success_criteria`.
- **Crítico**: card destacado com o último veredicto (`succeeded`, `next_action`, `evidence`, `feedback`), com histórico recolhível dos anteriores.
- **Tool log**: stream de cada tool call (tool, step, iter, params, output do Gemma quando aplicável, result).

## Protocolo SSE

O hook `useAgentStream` faz `POST /run` com `{task}` e consome o stream `text/event-stream`. Eventos: `plan`, `step_start`, `tool_call`, `critic`, `router_idle`, `done`, `abort`, `error`, `close`.

## Build de produção

```bash
npm run build
npm run preview
```
