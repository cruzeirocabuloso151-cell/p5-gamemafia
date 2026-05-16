"""Bridge between Llama (planner+router+critic) and Gemma (executor) on LM Studio.

Architecture: Plan-and-Execute with critique loop.
    1. Planner (Llama, mode=plan)     -> high-level plan of steps
    2. For each step:
       a. Step router (Llama, mode=step)   -> tool calls for this step
       b. Execute calls (Gemma when code-gen needed)
       c. Critic (Llama, mode=critique)    -> step_succeeded? continue|replan|abort
       d. If replan -> back to (1) with feedback
    3. Finish.

Usage:
    python3 bridge.py "sua tarefa aqui"

Both LM Studio servers must be running:
  - Llama  (planner+router+critic) on http://localhost:1234
  - Gemma  (executor)              on http://localhost:1235
"""

import json
import sys
import urllib.error
import urllib.request

LLAMA_URL = "http://localhost:1234/v1/chat/completions"
GEMMA_URL = "http://localhost:1235/v1/chat/completions"
LLAMA_MODEL = "local-llama"
GEMMA_MODEL = "local-gemma"

MAX_STEP_ITERS = 8        # router turns per step before forcing critique
MAX_REPLANS = 3           # how many times we accept replan before aborting
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


def _call_llama_json(prompt, max_tokens):
    raw = _post_chat(
        LLAMA_URL, LLAMA_MODEL, prompt,
        temperature=0.05, max_tokens=max_tokens, json_mode=True,
    )
    try:
        return json.loads(raw)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"Llama devolveu JSON inválido: {raw[:300]}") from e


def call_planner(task, context=None):
    prompt = f"mode: plan\ntask: {task}\n"
    if context:
        prompt += f"context: {json.dumps(context, ensure_ascii=False)}\n"
    plan = _call_llama_json(prompt, max_tokens=1024)
    if "plan" not in plan or not isinstance(plan["plan"], list):
        raise RuntimeError(f"planner devolveu sem 'plan': {plan}")
    return plan


def call_step_router(task, step, browser_state, history):
    prompt = (
        f"mode: step\n"
        f"task: {task}\n"
        f"current_step: {json.dumps(step, ensure_ascii=False)}\n"
        f"browser_state: {json.dumps(browser_state, ensure_ascii=False)}\n"
        f"history: {json.dumps(history, ensure_ascii=False)}"
    )
    decision = _call_llama_json(prompt, max_tokens=768)
    if "calls" not in decision:
        raise RuntimeError(f"router devolveu sem 'calls': {decision}")
    return decision


def call_critic(task, step, step_history, browser_state, vars_dict):
    prompt = (
        f"mode: critique\n"
        f"task: {task}\n"
        f"step: {json.dumps(step, ensure_ascii=False)}\n"
        f"step_history: {json.dumps(step_history, ensure_ascii=False)}\n"
        f"final_state: {json.dumps({'browser_state': browser_state, 'vars': vars_dict}, ensure_ascii=False)}"
    )
    verdict = _call_llama_json(prompt, max_tokens=512)
    for key in ("step_succeeded", "next_action"):
        if key not in verdict:
            raise RuntimeError(f"critic devolveu sem '{key}': {verdict}")
    return verdict


def call_executor(tool, params, step_context=None):
    parts = [f"tool: {tool}", f"params: {json.dumps(params, ensure_ascii=False)}"]
    if step_context:
        parts.append(f"step_context: {json.dumps(step_context, ensure_ascii=False)}")
    prompt = "\n".join(parts)
    return _post_chat(
        GEMMA_URL, GEMMA_MODEL, prompt,
        temperature=0.3, max_tokens=2048,
    )


# --- Tool handlers ---------------------------------------------------------
# Stubs: replace each body with real Playwright/Selenium/CDP calls.

def _h_navigate(params, _exec_out, _vars, browser_state):
    browser_state["url"] = params["url"]
    return {"status": 200, "url": params["url"]}


def _h_click(params, _exec_out, _vars, _browser_state):
    return {"ok": True, "selector": params["selector"]}


def _h_type(params, _exec_out, _vars, _browser_state):
    return {"ok": True}


def _h_eval_js(_params, exec_out, _vars, _browser_state):
    # exec_out is the JS snippet from Gemma; run it via CDP:
    # result = page.evaluate(exec_out)
    return {"snippet": exec_out, "result": None}


def _h_extract(_params, exec_out, _vars, _browser_state):
    try:
        return json.loads(exec_out)
    except json.JSONDecodeError:
        return {"raw": exec_out, "error": "executor JSON inválido"}


def _h_wait(params, _exec_out, _vars, _browser_state):
    return {"ok": True}


def _h_store(params, _exec_out, vars_dict, _browser_state):
    vars_dict[params["key"]] = params["value"]
    return {"ok": True}


def _h_respond_user(params, exec_out, _vars, _browser_state):
    message = exec_out if exec_out else params["message"]
    print(f"[agente] {message}")
    return {"ok": True, "message": message}


def _h_step_done(params, _exec_out, _vars, _browser_state):
    return {"__step_done__": True, "evidence": params.get("evidence")}


def _h_finish(params, _exec_out, _vars, _browser_state):
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
    "step_done": _h_step_done,
    "finish": _h_finish,
}

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


def _execute_calls(calls, vars_dict, browser_state, step_context, step_history, global_history, iteration):
    """Run one batch of tool calls respecting depends_on. Returns 'step_done'|'finish'|None."""
    done = set()
    remaining = list(calls)
    while remaining:
        progressed = False
        for call in list(remaining):
            if not all(dep in done for dep in call.get("depends_on", [])):
                continue
            tool = call["tool"]
            params = _resolve_refs(call.get("params", {}), vars_dict)

            exec_out = None
            if tool in EXECUTOR_TOOLS:
                exec_out = call_executor(tool, params, step_context)

            handler = TOOL_HANDLERS.get(tool)
            if handler is None:
                result = {"error": f"tool desconhecida: {tool}"}
            else:
                result = handler(params, exec_out, vars_dict, browser_state)

            save_as = call.get("save_as")
            if save_as:
                vars_dict[save_as] = result

            entry = {
                "iter": iteration,
                "id": call["id"],
                "tool": tool,
                "params": params,
                "result": result,
            }
            step_history.append(entry)
            global_history.append(entry)

            if isinstance(result, dict):
                if result.get("__finish__"):
                    return "finish"
                if result.get("__step_done__"):
                    return "step_done"

            done.add(call["id"])
            remaining.remove(call)
            progressed = True
        if not progressed:
            raise RuntimeError(f"ciclo em depends_on: {[c['id'] for c in remaining]}")
    return None


def _run_step(task, step, browser_state, vars_dict, global_history):
    """Run a single step until step_done, finish, or iteration cap."""
    step_history = []
    for it in range(MAX_STEP_ITERS):
        decision = call_step_router(task, step, browser_state, step_history)
        calls = decision.get("calls", [])
        if not calls:
            print(f"[router] step {step['id']} sem ações: {decision.get('reason', '')}")
            break
        outcome = _execute_calls(
            calls, vars_dict, browser_state, step, step_history, global_history, it,
        )
        if outcome in ("step_done", "finish"):
            return outcome, step_history
    return "iter_cap", step_history


def run(task):
    browser_state = {"url": "about:blank"}
    vars_dict = {}
    global_history = []

    plan = call_planner(task)
    print(f"[planner] {plan.get('reason', '')}")
    for s in plan["plan"]:
        print(f"  - {s['id']}: {s['goal']}")

    replans = 0
    step_index = 0
    steps = plan["plan"]

    while step_index < len(steps):
        step = steps[step_index]
        print(f"[step {step['id']}] {step['goal']}")
        outcome, step_history = _run_step(task, step, browser_state, vars_dict, global_history)

        if outcome == "finish":
            print("[finish] sessão encerrada pelo router")
            return

        verdict = call_critic(task, step, step_history, browser_state, vars_dict)
        print(f"[critic] {step['id']} succeeded={verdict['step_succeeded']} "
              f"action={verdict['next_action']} — {verdict.get('reason', '')}")

        action = verdict["next_action"]
        if action == "abort":
            print(f"[abort] {verdict.get('reason', 'sem motivo')}")
            return
        if action == "replan":
            replans += 1
            if replans > MAX_REPLANS:
                print(f"[abort] replan limit ({MAX_REPLANS}) excedido")
                return
            context = {
                "previous_plan": [s["goal"] for s in steps],
                "failed_at_step": step["id"],
                "feedback": verdict.get("feedback_for_planner", ""),
                "browser_state": browser_state,
                "recent_history": global_history[-15:],
            }
            plan = call_planner(task, context=context)
            print(f"[planner] replan #{replans}: {plan.get('reason', '')}")
            steps = plan["plan"]
            step_index = 0
            continue

        # next_action == "continue"
        if not verdict["step_succeeded"] and outcome == "iter_cap":
            print(f"[warn] step {step['id']} estourou {MAX_STEP_ITERS} iters sem step_done; crítico mandou seguir")
        step_index += 1

    print("[done] plano concluído")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    run(sys.argv[1])
