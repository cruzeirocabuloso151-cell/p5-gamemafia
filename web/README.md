# LM Studio Agent — Web Client

UI React/Vite/TS para o agente Plan-and-Execute do `lmstudio_prompts/bridge.py`.

## Como rodar localmente (dev)

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

- **Endpoint**: por padrão `http://127.0.0.1:8000` (editável no canto superior direito).
- **Task input**: textarea para descrever a tarefa em PT. Atalhos: `Ctrl+Enter` rodar, `Esc` cancelar, `Ctrl+Shift+T` toggle tema.
- **Plan board**: cada step do planejador vira um card colorido por status (pendente / rodando / sucesso / falhou / replan).
- **Browser preview**: screenshots ao vivo após cada ação que muda estado da página (precisa de Playwright instalado).
- **Crítico**: card destacado com o último veredicto + histórico recolhível.
- **Tool log**: stream de cada tool call com botão de copiar.
- **Parâmetros**: sliders/inputs que escrevem em `POST /config` em tempo real (max_step_iters, max_replans, temperaturas).
- **Credenciais**: CRUD pra `<credencial:nome>` (valores nunca aparecem nos logs).
- **Export**: download JSON do run inteiro ou CSV das vars armazenadas.

## Protocolo SSE

O hook `useAgentStream` faz `POST /run` com `{task}` e consome `text/event-stream`. Eventos: `run_start`, `plan`, `step_start`, `tool_call`, `screenshot`, `critic`, `router_idle`, `vars_snapshot`, `done`, `abort`, `error`, `close`.

## Build de produção (local)

```bash
npm run build
npm run preview
```

## Deploy no GitHub Pages

O workflow `.github/workflows/pages.yml` faz build de `web/` e publica em
`https://<usuario>.github.io/p5-gamemafia/` em pushes para `main` ou para a
branch `claude/lmstudio-dual-model-setup-6I6iS`.

### Setup único (uma vez por repo)

1. No GitHub, abra **Settings → Pages**.
2. Em **Source**, escolha **GitHub Actions**.
3. Pronto. O próximo push que toca em `web/**` ou no workflow vai disparar o deploy.

### Como funciona

- `BASE_PATH=/p5-gamemafia/` é injetado no build (via env var) para que o Vite gere caminhos corretos para assets servidos sob subpath.
- Localmente (`npm run dev`/`build`), `BASE_PATH` fica vazio e o build usa `/`.
- O bridge **continua rodando local** na sua máquina (`localhost:8000`). A página hospedada faz `fetch` direto para `127.0.0.1` — browsers permitem isso porque loopback é tratado como "potentially trustworthy" mesmo a partir de HTTPS.

### Limitações

- **Não funciona em mobile/celular**: o seu telefone não tem o bridge rodando em `127.0.0.1`.
- **Não use IPs de LAN** (ex: `192.168.x.x`): browsers vão bloquear como mixed-content a partir de uma origem HTTPS. Se precisar acessar de outra máquina, use um túnel (ngrok, cloudflared) e configure CORS/auth.
- **CORS aberto**: o bridge aceita qualquer origem. OK pra uso local; pra exposição externa, adicione token bearer.
