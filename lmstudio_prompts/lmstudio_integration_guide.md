# Guia de Integração — Dois Modelos no LM Studio

Este guia descreve como rodar **Gemma 3** (thinker: planner + router + critic) e **Qwen** (executor) simultaneamente no LM Studio, expondo cada um em uma porta diferente para que o `bridge.py` consiga orquestrá-los no padrão **Plan-and-Execute** com camada de crítica.

## Arquitetura em uma figura

```
                    ┌──────────────────────────────────────────┐
                    │             Gemma 3 (porta 1234)          │
                    │  mode=plan  ┐                             │
   task ─────────►  │  mode=step  ├─ JSON estruturado            │
                    │  mode=critique ┘                          │
                    └──────────────────────────────────────────┘
                                  ▲          │
                                  │          ▼
                          ┌───────┴──────────────┐
                          │    bridge.py         │  ─ tool handlers ─►  Playwright (browser real)
                          │  (orquestrador)      │
                          └──────────┬───────────┘
                                     │ (eval_js, extract, respond_user)
                                     ▼
                    ┌──────────────────────────────────────────┐
                    │             Qwen (porta 1235)             │
                    │   gera JS / JSON / texto concreto         │
                    └──────────────────────────────────────────┘
```

Loop por step: `step-router → executor + tools → critic → continue|replan|abort`.

## Pré-requisitos

- LM Studio ≥ 0.3.x (suporta múltiplos servidores locais simultâneos).
- (Opcional, recomendado) Playwright Python para browser real:
  ```bash
  pip install playwright pillow
  playwright install chromium
  ```
  Sem isso, as tool handlers caem para stubs e o painel "Browser preview" da UI fica vazio.

- RAM/VRAM mínima sugerida por quantização:

| Modelo                    | Q4_K_M  | Q6_K    | Q8_0    |
|---------------------------|---------|---------|---------|
| Gemma 3 4B                | ~3 GB   | ~4 GB   | ~5 GB   |
| Gemma 3 12B               | ~8 GB   | ~10 GB  | ~13 GB  |
| Gemma 3 27B               | ~17 GB  | ~22 GB  | ~29 GB  |
| Qwen 2.5 7B Instruct      | ~5 GB   | ~6 GB   | ~8 GB   |
| Qwen 3 8B                 | ~6 GB   | ~7 GB   | ~9 GB   |
| Qwen 2.5 14B Instruct     | ~9 GB   | ~11 GB  | ~15 GB  |

> "qwen9b" não é um tamanho canônico — escolha o Qwen disponível no seu LM Studio (Qwen 3 8B é o mais próximo). Ajuste `EXECUTOR_MODEL` em `bridge.py` para casar com o `model` que o LM Studio reporta.

Combinações sugeridas:

- **16 GB VRAM**: Gemma 3 4B Q6 + Qwen 3 8B Q4_K_M
- **24 GB VRAM**: Gemma 3 12B Q4 + Qwen 2.5 14B Q4
- **CPU only**: Gemma 3 4B Q4 + Qwen 3 8B Q4 (lento, mas funciona)

## Carregando os dois modelos

### Passo 1 — Gemma 3 (porta 1234) — thinker

Único servidor Gemma atende os três modos (plan / step / critique). O modo é selecionado pelo prefixo `mode:` no input enviado pelo bridge.

1. Abra a aba **Developer** → **Local Server**.
2. Em **Select a model to load**, escolha o Gemma 3 baixado.
3. Em **Server Port**, deixe `1234` (default).
4. Em **System Prompt**, cole o conteúdo de `gemma3_thinker.md` (já inclui as instruções dos 3 modos).
5. Clique **Start Server**.

### Passo 2 — Qwen (porta 1235) — executor

1. **Sem parar o servidor anterior**, abra uma nova instância de servidor (botão "+" na lista de servidores, ou abra uma nova janela do LM Studio).
2. Carregue o Qwen.
3. Em **Server Port**, mude para `1235`.
4. Em **System Prompt**, cole o conteúdo de `qwen_executor.md`.
5. Clique **Start Server**.

Se a versão do seu LM Studio não permite duas instâncias na mesma janela, abra duas janelas — cada uma carrega seu próprio modelo e sobe seu próprio servidor.

## Parâmetros recomendados

### Gemma 3 (porta 1234) — todos os modos

| Parâmetro         | Valor   | Por quê                                             |
|-------------------|---------|-----------------------------------------------------|
| `temperature`     | `0.05`  | JSON puro exige determinismo                        |
| `top_p`           | `0.9`   | Cuts long tail                                      |
| `max_tokens`      | `1024`  | `plan` precisa de mais espaço que `step`/`critique` |
| `response_format` | `json_object` (se suportado) | Garante JSON válido           |
| `stop`            | (vazio) | Não trunque o JSON                                  |

O `bridge.py` já manda `max_tokens` específico por modo (1024 plan, 768 step, 512 critique); o valor da UI é só o teto. O `temperature` é ajustável em tempo real pela UI (`router_temperature`).

### Qwen (porta 1235) — executor

| Parâmetro     | Valor   | Por quê                                                  |
|---------------|---------|----------------------------------------------------------|
| `temperature` | `0.3`   | Permite alguma variação em código sem alucinar           |
| `top_p`       | `0.95`  | Vocabulário técnico amplo                                |
| `max_tokens`  | `2048`  | Snippets JS / extrações grandes cabem aqui               |

## Validação rápida

Com ambos os servidores no ar, valide com `curl`:

**Thinker — modo `plan`:**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-3",
    "messages": [
      {"role": "user", "content": "mode: plan\ntask: abrir https://example.com e extrair o título"}
    ],
    "temperature": 0.05,
    "max_tokens": 1024,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: `content` é JSON com chave `plan` (lista de steps com `id`, `goal`, `success_criteria`).

**Thinker — modo `step`:**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-3",
    "messages": [
      {"role": "user", "content": "mode: step\ntask: abrir https://example.com\ncurrent_step: {\"id\":\"s1\",\"goal\":\"navegar\",\"success_criteria\":\"url == https://example.com\"}\nbrowser_state: {\"url\":\"about:blank\"}\nhistory: []"}
    ],
    "temperature": 0.05,
    "max_tokens": 768,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: JSON com chave `calls` contendo `navigate` + `step_done`.

**Thinker — modo `critique`:**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemma-3",
    "messages": [
      {"role": "user", "content": "mode: critique\ntask: abrir https://example.com\nstep: {\"id\":\"s1\",\"goal\":\"navegar\",\"success_criteria\":\"url == https://example.com\"}\nstep_history: [{\"tool\":\"navigate\",\"result\":{\"status\":200,\"url\":\"https://example.com\"}}]\nfinal_state: {\"browser_state\":{\"url\":\"https://example.com\"},\"vars\":{}}"}
    ],
    "temperature": 0.05,
    "max_tokens": 512,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: JSON com `step_succeeded: true`, `next_action: "continue"` e `evidence` citando a URL.

**Executor (Qwen):**
```bash
curl -s http://localhost:1235/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen",
    "messages": [
      {"role": "user", "content": "tool: eval_js\nparams: {\"goal\": \"pegar o título da página\"}"}
    ],
    "temperature": 0.3,
    "max_tokens": 2048
  }'
```

Esperado: o `content` da resposta é `return document.title;` (ou variação equivalente), **sem** cercas de código.

## Endpoints do bridge

O `bridge.py --serve` expõe:

| Método  | Path                  | Função                                       |
|---------|-----------------------|----------------------------------------------|
| GET     | `/`                   | Healthcheck + status do Playwright           |
| POST    | `/run`                | SSE stream do agente (`{"task": "..."}`)     |
| GET     | `/config`             | Lê config atual                              |
| POST    | `/config`             | Patch parcial: `max_step_iters`, `max_replans`, `router_temperature`, `executor_temperature`, `screenshot_max_width` |
| GET     | `/credentials`        | Lista nomes (não valores) de credenciais     |
| POST    | `/credentials`        | Merge: `{"user": "alice", "pass": "secret"}` |
| DELETE  | `/credentials/<nome>` | Remove uma credencial                        |

Credenciais ficam em `lmstudio_prompts/.credentials.json` (chmod 600). O bridge resolve `<credencial:nome>` em qualquer string de params antes de chamar o handler, e **redige** os valores nos eventos SSE de volta (substituindo pelo placeholder), pra que o log/UI nunca exiba o segredo em texto claro.

## Troubleshooting

| Sintoma                                          | Causa provável                          | Correção                                                            |
|--------------------------------------------------|-----------------------------------------|---------------------------------------------------------------------|
| Thinker devolve texto antes do JSON              | Temperature muito alta                  | Baixe para `0.0`–`0.05`. Ative `response_format: json_object`.      |
| Thinker devolve JSON inválido (vírgula sobrando) | Modelo pequeno demais                   | Suba para Gemma 3 12B+; ou ative JSON-mode no LM Studio.            |
| Critic sempre devolve `step_succeeded: true`     | Modelo concordando demais               | Adicione no prompt do crítico: "se em dúvida, prefira `false`".     |
| Replans infinitos                                 | `feedback_for_planner` vago             | Verifique se o critic está citando evidência concreta; ajuste `max_replans` na UI. |
| Executor (Qwen) devolve com ```` ``` ```` ao redor | Prompt não foi colado direito         | Reabra a aba System Prompt e cole de novo. Reinicie o servidor.     |
| VRAM insuficiente ao carregar o segundo modelo   | Quant alta demais                       | Use Q4_K_M para o thinker; mantenha o executor em quant maior.      |
| Latência alta (>10s por turno)                   | Ambos os modelos disputando GPU         | Force o thinker em CPU (config do LM Studio → "CPU Only").          |
| `bridge.py` recebe 404                           | Porta errada ou servidor não iniciado   | Confirme com `curl http://localhost:1234/v1/models`.                |
| Modelo responde em inglês mesmo com prompt em PT | Falta de instrução explícita            | Adicione "Responda sempre em português." no system prompt.          |
| Painel browser preview vazio                     | Playwright não instalado                | `pip install playwright pillow && playwright install chromium`.     |
