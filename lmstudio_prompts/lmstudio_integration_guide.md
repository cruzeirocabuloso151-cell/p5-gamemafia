# Guia de Integração — Dois Modelos no LM Studio

Este guia descreve como rodar **Llama Instruct** (planner + router + critic) e **Gemma 3** (executor) simultaneamente no LM Studio, expondo cada um em uma porta diferente para que o `bridge.py` consiga orquestrá-los no padrão **Plan-and-Execute** com camada de crítica.

## Arquitetura em uma figura

```
                    ┌──────────────────────────────────────────┐
                    │              Llama (porta 1234)           │
                    │  mode=plan  ┐                             │
   task ─────────►  │  mode=step  ├─ JSON estruturado            │
                    │  mode=critique ┘                          │
                    └──────────────────────────────────────────┘
                                  ▲          │
                                  │          ▼
                          ┌───────┴──────────────┐
                          │    bridge.py         │  ─ tool handlers ─►  Playwright/CDP
                          │  (orquestrador)      │
                          └──────────┬───────────┘
                                     │ (eval_js, extract, respond_user)
                                     ▼
                    ┌──────────────────────────────────────────┐
                    │           Gemma 3 (porta 1235)            │
                    │   gera JS / JSON / texto concreto         │
                    └──────────────────────────────────────────┘
```

Loop por step: `step-router → executor + tools → critic → continue|replan|abort`.

## Pré-requisitos

- LM Studio ≥ 0.3.x (suporta múltiplos servidores locais simultâneos).
- RAM/VRAM mínima sugerida por quantização:

| Modelo                    | Q4_K_M  | Q6_K    | Q8_0    |
|---------------------------|---------|---------|---------|
| Llama 3.1 8B Instruct     | ~6 GB   | ~7 GB   | ~9 GB   |
| Gemma 3 4B                | ~3 GB   | ~4 GB   | ~5 GB   |
| Gemma 3 12B               | ~8 GB   | ~10 GB  | ~13 GB  |
| Gemma 3 27B               | ~17 GB  | ~22 GB  | ~29 GB  |

Para máquinas com 16 GB de VRAM, a combinação confortável é **Llama 8B Q4_K_M + Gemma 3 4B Q6_K**. Para 24 GB+, suba o Gemma para 12B.

## Carregando os dois modelos

### Passo 1 — Llama (porta 1234)

Único servidor Llama atende os três modos (plan / step / critique). O modo é selecionado pelo prefixo `mode:` no input enviado pelo bridge.

1. Abra a aba **Developer** → **Local Server**.
2. Em **Select a model to load**, escolha o Llama Instruct baixado.
3. Em **Server Port**, deixe `1234` (default).
4. Em **System Prompt**, cole o conteúdo de `llama_tool_router.md` (já inclui as instruções dos 3 modos).
5. Clique **Start Server**.

### Passo 2 — Gemma Executor (porta 1235)

1. **Sem parar o servidor anterior**, abra uma nova instância de servidor (botão "+" na lista de servidores, ou abra uma nova janela do LM Studio).
2. Carregue o Gemma 3.
3. Em **Server Port**, mude para `1235`.
4. Em **System Prompt**, cole o conteúdo de `gemma3_agent_executor.md`.
5. Clique **Start Server**.

Se a versão do seu LM Studio não permite duas instâncias na mesma janela, abra duas janelas — cada uma carrega seu próprio modelo e sobe seu próprio servidor.

## Parâmetros recomendados

### Llama (porta 1234) — todos os modos

| Parâmetro         | Valor   | Por quê                                             |
|-------------------|---------|-----------------------------------------------------|
| `temperature`     | `0.05`  | JSON puro exige determinismo                        |
| `top_p`           | `0.9`   | Cuts long tail                                      |
| `max_tokens`      | `1024`  | `plan` precisa de mais espaço que `step`/`critique` |
| `response_format` | `json_object` (se suportado) | Garante JSON válido           |
| `stop`            | (vazio) | Não trunque o JSON                                  |

O `bridge.py` já manda `max_tokens` específico por modo (1024 plan, 768 step, 512 critique); o valor da UI é só o teto.

### Executor (Gemma, porta 1235)

| Parâmetro     | Valor   | Por quê                                                  |
|---------------|---------|----------------------------------------------------------|
| `temperature` | `0.3`   | Permite alguma variação em código sem alucinar           |
| `top_p`       | `0.95`  | Vocabulário técnico amplo                                |
| `max_tokens`  | `2048`  | Snippets JS / extrações grandes cabem aqui               |

## Validação rápida

Com ambos os servidores no ar, valide com `curl`:

**Planner (modo `plan`):**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-llama",
    "messages": [
      {"role": "user", "content": "mode: plan\ntask: abrir https://example.com e extrair o título"}
    ],
    "temperature": 0.05,
    "max_tokens": 1024,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: `content` é JSON com chave `plan` (lista de steps com `id`, `goal`, `success_criteria`).

**Step router (modo `step`):**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-llama",
    "messages": [
      {"role": "user", "content": "mode: step\ntask: abrir https://example.com\ncurrent_step: {\"id\":\"s1\",\"goal\":\"navegar\",\"success_criteria\":\"url == https://example.com\"}\nbrowser_state: {\"url\":\"about:blank\"}\nhistory: []"}
    ],
    "temperature": 0.05,
    "max_tokens": 768,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: JSON com chave `calls` contendo `navigate` + `step_done`.

**Critic (modo `critique`):**
```bash
curl -s http://localhost:1234/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-llama",
    "messages": [
      {"role": "user", "content": "mode: critique\ntask: abrir https://example.com\nstep: {\"id\":\"s1\",\"goal\":\"navegar\",\"success_criteria\":\"url == https://example.com\"}\nstep_history: [{\"tool\":\"navigate\",\"result\":{\"status\":200,\"url\":\"https://example.com\"}}]\nfinal_state: {\"browser_state\":{\"url\":\"https://example.com\"},\"vars\":{}}"}
    ],
    "temperature": 0.05,
    "max_tokens": 512,
    "response_format": {"type": "json_object"}
  }'
```

Esperado: JSON com `step_succeeded: true`, `next_action: "continue"` e `evidence` citando a URL.

**Executor:**
```bash
curl -s http://localhost:1235/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "local-gemma",
    "messages": [
      {"role": "user", "content": "tool: eval_js\nparams: {\"goal\": \"pegar o título da página\"}"}
    ],
    "temperature": 0.3,
    "max_tokens": 2048
  }'
```

Esperado: o `content` da resposta é `return document.title;` (ou variação equivalente), **sem** cercas de código.

## Troubleshooting

| Sintoma                                          | Causa provável                          | Correção                                                            |
|--------------------------------------------------|-----------------------------------------|---------------------------------------------------------------------|
| Llama devolve texto antes do JSON                | Temperature muito alta                  | Baixe para `0.0`–`0.05`. Ative `response_format: json_object`.      |
| Llama devolve JSON inválido (vírgula sobrando)   | Modelo pequeno demais                   | Suba para Llama 70B Q3, ou troque para um modelo treinado em JSON.  |
| Critic sempre devolve `step_succeeded: true`     | Modelo concordando demais               | Adicione no prompt do crítico: "se em dúvida, prefira `false`".     |
| Replans infinitos                                 | `feedback_for_planner` vago             | Verifique se o critic está citando evidência concreta; suba o cap `MAX_REPLANS` só se necessário. |
| Executor devolve com ```` ``` ```` ao redor       | Prompt não foi colado direito           | Reabra a aba System Prompt e cole de novo. Reinicie o servidor.     |
| VRAM insuficiente ao carregar o segundo modelo   | Quant alta demais                       | Use Q4_K_M para o router; mantenha o executor em quant maior.       |
| Latência alta (>10s por turno)                   | Ambos os modelos disputando GPU         | Force o router em CPU (config do LM Studio → "CPU Only").           |
| `bridge.py` recebe 404                           | Porta errada ou servidor não iniciado   | Confirme com `curl http://localhost:1234/v1/models`.                |
| Modelo responde em inglês mesmo com prompt em PT | Falta de instrução explícita            | Adicione "Responda sempre em português." no system prompt.          |
