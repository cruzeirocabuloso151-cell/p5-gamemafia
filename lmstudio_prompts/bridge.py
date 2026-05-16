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
    python3 bridge.py "sua tarefa aqui"         # CLI: prints events to stdout
    python3 bridge.py --serve [port]            # HTTP server with SSE (default 8000)

LM Studio servers expected:
  - Llama  (planner+router+critic) on http://localhost:1234
  - Gemma  (executor)              on http://localhost:1235
"""

import json
import sys
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LLAMA_URL = "http://localhost:1234/v1/chat/completions"
GEMMA_URL = "http://localhost:1235/v1/chat/completions"
LLAMA_MODEL = "local-llama"
GEMMA_MODEL = "local-gemma"

MAX_STEP_ITERS = 8
MAX_REPLANS = 3
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

def _h_navigate(params, _exec_out, _vars, browser_state):
    browser_state["url"] = params["url"]
    return {"status": 200, "url": params["url"]}


def _h_click(params, _exec_out, _vars, _browser_state):
    return {"ok": True, "selector": params["selector"]}


def _h_type(params, _exec_out, _vars, _browser_state):
    return {"ok": True}


def _h_eval_js(_params, exec_out, _vars, _browser_state):
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


# --- Event-emitting core ---------------------------------------------------
# run_events(task) is a generator that yields ("event_name", payload_dict).
# Used both by the CLI wrapper and by the SSE server.

def _execute_calls(calls, vars_dict, browser_state, step, step_history, global_history, iteration):
    done = set()
    remaining = list(calls)
    events = []
    terminal = None
    while remaining:
        progressed = False
        for call in list(remaining):
            if not all(dep in done for dep in call.get("depends_on", [])):
                continue
            tool = call["tool"]
            params = _resolve_refs(call.get("params", {}), vars_dict)

            exec_out = None
            if tool in EXECUTOR_TOOLS:
                exec_out = call_executor(tool, params, step)

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
                "executor_output": exec_out,
            }
            step_history.append(entry)
            global_history.append(entry)
            events.append(("tool_call", {"step_id": step["id"], **entry}))

            if isinstance(result, dict):
                if result.get("__finish__"):
                    terminal = "finish"
                    break
                if result.get("__step_done__"):
                    terminal = "step_done"
                    break

            done.add(call["id"])
            remaining.remove(call)
            progressed = True
        if terminal:
            break
        if not progressed:
            raise RuntimeError(f"ciclo em depends_on: {[c['id'] for c in remaining]}")
    return events, terminal


def run_events(task):
    browser_state = {"url": "about:blank"}
    vars_dict = {}
    global_history = []

    try:
        plan = call_planner(task)
    except Exception as e:
        yield ("error", {"message": str(e), "where": "planner"})
        return
    yield ("plan", {"steps": plan["plan"], "reason": plan.get("reason", ""), "replan_count": 0})

    replans = 0
    step_index = 0
    steps = plan["plan"]

    while step_index < len(steps):
        step = steps[step_index]
        yield ("step_start", {"step_id": step["id"], "goal": step["goal"],
                              "success_criteria": step.get("success_criteria", "")})
        step_history = []
        terminal = None
        for it in range(MAX_STEP_ITERS):
            try:
                decision = call_step_router(task, step, browser_state, step_history)
            except Exception as e:
                yield ("error", {"message": str(e), "where": f"router/{step['id']}"})
                return
            calls = decision.get("calls", [])
            if not calls:
                yield ("router_idle", {"step_id": step["id"], "reason": decision.get("reason", "")})
                break
            try:
                events, terminal = _execute_calls(
                    calls, vars_dict, browser_state, step, step_history, global_history, it,
                )
            except Exception as e:
                yield ("error", {"message": str(e), "where": f"exec/{step['id']}"})
                return
            for ev in events:
                yield ev
            if terminal:
                break

        if terminal == "finish":
            yield ("done", {"reason": "finish emitido pelo router"})
            return

        try:
            verdict = call_critic(task, step, step_history, browser_state, vars_dict)
        except Exception as e:
            yield ("error", {"message": str(e), "where": f"critic/{step['id']}"})
            return
        yield ("critic", {
            "step_id": step["id"],
            "succeeded": verdict["step_succeeded"],
            "evidence": verdict.get("evidence", ""),
            "next_action": verdict["next_action"],
            "feedback": verdict.get("feedback_for_planner", ""),
            "reason": verdict.get("reason", ""),
        })

        action = verdict["next_action"]
        if action == "abort":
            yield ("abort", {"reason": verdict.get("reason", "sem motivo")})
            return
        if action == "replan":
            replans += 1
            if replans > MAX_REPLANS:
                yield ("abort", {"reason": f"replan limit ({MAX_REPLANS}) excedido"})
                return
            context = {
                "previous_plan": [s["goal"] for s in steps],
                "failed_at_step": step["id"],
                "feedback": verdict.get("feedback_for_planner", ""),
                "browser_state": browser_state,
                "recent_history": global_history[-15:],
            }
            try:
                plan = call_planner(task, context=context)
            except Exception as e:
                yield ("error", {"message": str(e), "where": "planner/replan"})
                return
            yield ("plan", {"steps": plan["plan"], "reason": plan.get("reason", ""),
                            "replan_count": replans})
            steps = plan["plan"]
            step_index = 0
            continue

        step_index += 1

    yield ("done", {"reason": "plano concluído"})


def run(task):
    """CLI: consume events and print them."""
    for name, payload in run_events(task):
        if name == "plan":
            print(f"[planner] {payload['reason']}")
            for s in payload["steps"]:
                print(f"  - {s['id']}: {s['goal']}")
        elif name == "step_start":
            print(f"[step {payload['step_id']}] {payload['goal']}")
        elif name == "tool_call":
            print(f"  · {payload['tool']} → {json.dumps(payload['result'], ensure_ascii=False)[:120]}")
        elif name == "critic":
            mark = "ok" if payload["succeeded"] else "fail"
            print(f"[critic/{mark}] {payload['step_id']} action={payload['next_action']} — {payload['reason']}")
        elif name == "router_idle":
            print(f"[router] idle: {payload['reason']}")
        elif name in ("done", "abort", "error"):
            print(f"[{name}] {payload}")


# --- SSE server ------------------------------------------------------------

CORS_HEADERS = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
}


class _AgentHandler(BaseHTTPRequestHandler):
    def _set_cors(self):
        for k, v in CORS_HEADERS.items():
            self.send_header(k, v)

    def do_OPTIONS(self):
        self.send_response(204)
        self._set_cors()
        self.end_headers()

    def do_GET(self):
        if self.path == "/":
            self.send_response(200)
            self._set_cors()
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.end_headers()
            self.wfile.write(b"bridge.py SSE server. POST /run {task}")
        else:
            self.send_response(404)
            self._set_cors()
            self.end_headers()

    def do_POST(self):
        if self.path != "/run":
            self.send_response(404)
            self._set_cors()
            self.end_headers()
            return

        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length else b"{}"
        try:
            body = json.loads(raw.decode("utf-8"))
            task = body["task"]
        except (json.JSONDecodeError, KeyError):
            self.send_response(400)
            self._set_cors()
            self.end_headers()
            self.wfile.write(b'{"error":"esperado body JSON com {\\"task\\": \\"...\\"}"}')
            return

        self.send_response(200)
        self._set_cors()
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.send_header("X-Accel-Buffering", "no")
        self.end_headers()

        def emit(event_name, payload):
            data = json.dumps(payload, ensure_ascii=False)
            chunk = f"event: {event_name}\ndata: {data}\n\n".encode("utf-8")
            try:
                self.wfile.write(chunk)
                self.wfile.flush()
                return True
            except (BrokenPipeError, ConnectionResetError):
                return False

        try:
            for name, payload in run_events(task):
                if not emit(name, payload):
                    return
            emit("close", {})
        except Exception as e:
            emit("error", {"message": str(e), "where": "stream"})

    def log_message(self, fmt, *args):
        sys.stderr.write(f"[server] {fmt % args}\n")


def serve(host="127.0.0.1", port=8000):
    server = ThreadingHTTPServer((host, port), _AgentHandler)
    print(f"bridge SSE em http://{host}:{port}  (POST /run com body {{\"task\":\"...\"}})")
    server.serve_forever()


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    if sys.argv[1] == "--serve":
        port = int(sys.argv[2]) if len(sys.argv) > 2 else 8000
        serve(port=port)
    else:
        run(sys.argv[1])
