"""Bridge between Llama (router) and Gemma (executor) running on LM Studio.

Usage:
    python3 bridge.py "sua tarefa aqui"

Both LM Studio servers must be running:
  - Router  (Llama Instruct) on http://localhost:1234
  - Executor (Gemma 3)       on http://localhost:1235
"""

import json
import sys
import urllib.error
import urllib.request

ROUTER_URL = "http://localhost:1234/v1/chat/completions"
EXECUTOR_URL = "http://localhost:1235/v1/chat/completions"
ROUTER_MODEL = "local-llama"
EXECUTOR_MODEL = "local-gemma"

MAX_ITERS = 12
HTTP_TIMEOUT = 120


def _post_chat(url, model, user_content, temperature, max_tokens, json_mode=False):
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": user_content}],
        "temperature": temperature,
        "max_tokens": max_tokens,
    }
    if json_mode:
        payload["response_format"] = {"type": "json_object"}
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}
    )
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT) as resp:
            body = json.loads(resp.read().decode("utf-8"))
    except urllib.error.URLError as e:
        raise RuntimeError(f"falha ao chamar {url}: {e}") from e
    return body["choices"][0]["message"]["content"]


def call_router(task, browser_state, history):
    prompt = (
        f"task: {task}\n"
        f"browser_state: {json.dumps(browser_state, ensure_ascii=False)}\n"
        f"history: {json.dumps(history, ensure_ascii=False)}"
    )
    raw = _post_chat(
        ROUTER_URL, ROUTER_MODEL, prompt,
        temperature=0.05, max_tokens=512, json_mode=True,
    )
    try:
        plan = json.loads(raw)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"router devolveu JSON inválido: {raw[:300]}") from e
    if "calls" not in plan:
        raise RuntimeError(f"router devolveu JSON sem 'calls': {plan}")
    return plan


def call_executor(tool, params, extra_context=None):
    parts = [f"tool: {tool}", f"params: {json.dumps(params, ensure_ascii=False)}"]
    if extra_context:
        parts.append(f"context: {json.dumps(extra_context, ensure_ascii=False)}")
    prompt = "\n".join(parts)
    return _post_chat(
        EXECUTOR_URL, EXECUTOR_MODEL, prompt,
        temperature=0.3, max_tokens=2048,
    )


# --- Tool handlers ---------------------------------------------------------
# Stubs: replace each body with real Playwright/Selenium/CDP calls.
# Every handler receives (params, executor_output, vars_dict) and returns
# whatever should be stored as the call's result.

def _h_navigate(params, _exec_out, _vars):
    # plug Playwright: page.goto(params["url"])
    return {"status": 200, "url": params["url"]}


def _h_click(params, _exec_out, _vars):
    # plug Playwright: page.click(params["selector"])
    return {"ok": True, "selector": params["selector"]}


def _h_type(params, _exec_out, _vars):
    # plug Playwright: page.fill(params["selector"], params["text"])
    return {"ok": True}


def _h_eval_js(_params, exec_out, _vars):
    # exec_out is the JS snippet from Gemma; run it via CDP:
    # result = page.evaluate(exec_out)
    return {"snippet": exec_out, "result": None}


def _h_extract(_params, exec_out, _vars):
    # Gemma returned JSON. Parse and store.
    try:
        return json.loads(exec_out)
    except json.JSONDecodeError:
        return {"raw": exec_out, "error": "executor JSON inválido"}


def _h_wait(params, _exec_out, _vars):
    # plug Playwright: page.wait_for_selector(params["selector"], timeout=params["timeout_ms"])
    return {"ok": True}


def _h_store(params, _exec_out, vars_dict):
    vars_dict[params["key"]] = params["value"]
    return {"ok": True}


def _h_respond_user(params, _exec_out, _vars):
    print(f"[agente] {params['message']}")
    return {"ok": True}


def _h_finish(params, _exec_out, _vars):
    return {"__finish__": True, "summary": params.get("summary", "")}


TOOL_HANDLERS = {
    "navigate": _h_navigate,
    "click": _h_click,
    "type": _h_type,
    "eval_js": _h_eval_js,
    "extract": _h_extract,
    "wait": _h_wait,
    "store": _h_store,
    "respond_user": _h_respond_user,
    "finish": _h_finish,
}

# Tools whose params/output need the executor to generate concrete code first.
EXECUTOR_TOOLS = {"eval_js", "extract", "respond_user"}


def _resolve_refs(value, vars_dict):
    """Replace '$varname' or '$varname.path' strings with values from vars_dict."""
    if isinstance(value, str) and value.startswith("$"):
        path = value[1:].split(".")
        cur = vars_dict.get(path[0])
        for key in path[1:]:
            if cur is None:
                return None
            cur = cur.get(key) if isinstance(cur, dict) else None
        return cur
    if isinstance(value, dict):
        return {k: _resolve_refs(v, vars_dict) for k, v in value.items()}
    if isinstance(value, list):
        return [_resolve_refs(v, vars_dict) for v in value]
    return value


def run(task):
    browser_state = {"url": "about:blank"}
    history = []
    vars_dict = {}

    for iteration in range(MAX_ITERS):
        plan = call_router(task, browser_state, history)
        calls = plan.get("calls", [])
        if not calls:
            print(f"[router] nada a fazer: {plan.get('reason', '')}")
            return

        # naive ordering: respect depends_on by sorting topologically
        done = set()
        remaining = list(calls)
        while remaining:
            progressed = False
            for call in list(remaining):
                if all(dep in done for dep in call.get("depends_on", [])):
                    tool = call["tool"]
                    params = _resolve_refs(call.get("params", {}), vars_dict)

                    exec_out = None
                    if tool in EXECUTOR_TOOLS:
                        exec_out = call_executor(tool, params)

                    handler = TOOL_HANDLERS.get(tool)
                    if handler is None:
                        result = {"error": f"tool desconhecida: {tool}"}
                    else:
                        result = handler(params, exec_out, vars_dict)

                    save_as = call.get("save_as")
                    if save_as:
                        vars_dict[save_as] = result

                    history.append({
                        "iter": iteration,
                        "id": call["id"],
                        "tool": tool,
                        "params": params,
                        "result": result,
                    })

                    if isinstance(result, dict) and result.get("__finish__"):
                        print(f"[finish] {result.get('summary', '')}")
                        return

                    done.add(call["id"])
                    remaining.remove(call)
                    progressed = True
            if not progressed:
                raise RuntimeError(f"ciclo em depends_on: {[c['id'] for c in remaining]}")

    print(f"[bridge] max_iters={MAX_ITERS} atingido sem finish")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    run(sys.argv[1])
