"""Bridge between Llama (planner+router+critic) and Gemma (executor) on LM Studio.

Architecture: Plan-and-Execute with critique loop + optional real browser.

    1. Planner (Llama, mode=plan)     -> high-level plan of steps
    2. For each step:
       a. Step router (Llama, mode=step)   -> tool calls for this step
       b. Execute calls (Gemma when code-gen needed; Playwright when available)
       c. Critic (Llama, mode=critique)    -> step_succeeded? continue|replan|abort
       d. If replan -> back to (1) with feedback
    3. Finish.

Usage:
    python3 bridge.py "sua tarefa aqui"         # CLI: prints events to stdout
    python3 bridge.py --serve [port]            # HTTP server with SSE (default 8000)

Optional dependency:
    pip install playwright && playwright install chromium
"""

import base64
import json
import os
import re
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

THINKER_URL = "http://localhost:1234/v1/chat/completions"
EXECUTOR_URL = "http://localhost:1235/v1/chat/completions"
THINKER_MODEL = "gemma-3"     # plan/step/critique — structured JSON reasoning
EXECUTOR_MODEL = "qwen"        # tool-call code generation

HTTP_TIMEOUT = 120
CREDENTIALS_PATH = os.environ.get(
    "BRIDGE_CREDENTIALS_PATH",
    os.path.join(os.path.dirname(__file__), ".credentials.json"),
)

# --- Runtime config (mutable via POST /config) -----------------------------

@dataclass
class RuntimeConfig:
    max_step_iters: int = 8
    max_replans: int = 3
    router_temperature: float = 0.05
    executor_temperature: float = 0.3
    screenshot_max_width: int = 1024
    enable_browser: bool = True

    def update(self, patch: dict):
        for k, v in patch.items():
            if hasattr(self, k):
                setattr(self, k, v)

    def to_dict(self):
        return {k: getattr(self, k) for k in self.__dataclass_fields__}


CONFIG = RuntimeConfig()
_CONFIG_LOCK = threading.Lock()

# --- Credentials -----------------------------------------------------------

_CREDS_LOCK = threading.Lock()
_CRED_RE = re.compile(r"<credencial:([^>]+)>")


def load_credentials() -> dict:
    if not os.path.exists(CREDENTIALS_PATH):
        return {}
    with _CREDS_LOCK, open(CREDENTIALS_PATH, "r", encoding="utf-8") as f:
        return json.load(f)


def save_credentials(creds: dict) -> None:
    with _CREDS_LOCK:
        with open(CREDENTIALS_PATH, "w", encoding="utf-8") as f:
            json.dump(creds, f, indent=2, ensure_ascii=False)
        os.chmod(CREDENTIALS_PATH, 0o600)


def _resolve_credentials(value, creds: dict):
    if isinstance(value, str):
        return _CRED_RE.sub(lambda m: str(creds.get(m.group(1), m.group(0))), value)
    if isinstance(value, dict):
        return {k: _resolve_credentials(v, creds) for k, v in value.items()}
    if isinstance(value, list):
        return [_resolve_credentials(v, creds) for v in value]
    return value


# --- LLM calls -------------------------------------------------------------

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


def _call_thinker_json(prompt, max_tokens):
    raw = _post_chat(
        THINKER_URL, THINKER_MODEL, prompt,
        temperature=CONFIG.router_temperature, max_tokens=max_tokens, json_mode=True,
    )
    try:
        return json.loads(raw)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"thinker devolveu JSON inválido: {raw[:300]}") from e


def call_planner(task, context=None):
    prompt = f"mode: plan\ntask: {task}\n"
    if context:
        prompt += f"context: {json.dumps(context, ensure_ascii=False)}\n"
    plan = _call_thinker_json(prompt, max_tokens=1024)
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
    decision = _call_thinker_json(prompt, max_tokens=768)
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
    verdict = _call_thinker_json(prompt, max_tokens=512)
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
        EXECUTOR_URL, EXECUTOR_MODEL, prompt,
        temperature=CONFIG.executor_temperature, max_tokens=2048,
    )


# --- Playwright browser (lazy + graceful fallback) -------------------------

class _Browser:
    """Thin wrapper around a Playwright sync page. Lazy-init on first use."""

    def __init__(self):
        self._page = None
        self._playwright = None
        self._browser = None
        self._available = None  # tri-state: None=untested, True=ok, False=missing

    def _ensure(self):
        if self._available is False:
            return False
        if self._page is not None:
            return True
        try:
            from playwright.sync_api import sync_playwright
        except ImportError:
            self._available = False
            return False
        self._playwright = sync_playwright().start()
        self._browser = self._playwright.chromium.launch(headless=True)
        self._page = self._browser.new_page()
        self._available = True
        return True

    @property
    def available(self):
        return self._available is True

    @property
    def page(self):
        return self._page

    def close(self):
        try:
            if self._browser:
                self._browser.close()
            if self._playwright:
                self._playwright.stop()
        except Exception:
            pass
        self._page = None
        self._browser = None
        self._playwright = None

    def screenshot(self, max_width):
        if not self._ensure() or not self._page:
            return None
        try:
            png = self._page.screenshot(type="png", full_page=False)
        except Exception:
            return None
        try:
            from io import BytesIO
            from PIL import Image  # type: ignore
            img = Image.open(BytesIO(png))
            if img.width > max_width:
                ratio = max_width / img.width
                img = img.resize((max_width, int(img.height * ratio)))
                out = BytesIO()
                img.save(out, format="PNG", optimize=True)
                png = out.getvalue()
        except ImportError:
            pass  # PIL absent -> serve full-resolution PNG
        return base64.b64encode(png).decode("ascii")


# --- Tool handlers ---------------------------------------------------------
# All handlers now receive (params, exec_out, ctx) where ctx exposes:
#   ctx.vars            mutable dict
#   ctx.browser_state   mutable dict
#   ctx.browser         _Browser
#   ctx.creds           resolved credentials dict (read-only)
#
# Handlers operating on the browser must call ctx.browser._ensure() and
# fall back to a stub result if not available.

class _Ctx:
    __slots__ = ("vars", "browser_state", "browser", "creds")

    def __init__(self, vars_dict, browser_state, browser, creds):
        self.vars = vars_dict
        self.browser_state = browser_state
        self.browser = browser
        self.creds = creds


def _h_navigate(params, _exec_out, ctx):
    url = params["url"]
    ctx.browser_state["url"] = url
    if ctx.browser._ensure():
        try:
            resp = ctx.browser.page.goto(url, wait_until="domcontentloaded", timeout=15000)
            return {"status": resp.status if resp else None, "url": ctx.browser.page.url}
        except Exception as e:
            return {"error": str(e), "url": url}
    return {"status": 200, "url": url, "stub": True}


def _h_click(params, _exec_out, ctx):
    sel = params["selector"]
    if ctx.browser._ensure():
        try:
            ctx.browser.page.click(sel, timeout=5000)
            return {"ok": True, "selector": sel}
        except Exception as e:
            return {"ok": False, "selector": sel, "error": str(e)}
    return {"ok": True, "selector": sel, "stub": True}


def _h_type(params, _exec_out, ctx):
    sel = params["selector"]
    text = params["text"]
    if ctx.browser._ensure():
        try:
            ctx.browser.page.fill(sel, text, timeout=5000)
            return {"ok": True}
        except Exception as e:
            return {"ok": False, "error": str(e)}
    return {"ok": True, "stub": True}


def _h_eval_js(_params, exec_out, ctx):
    if exec_out is None:
        return {"error": "executor não devolveu snippet"}
    if ctx.browser._ensure():
        try:
            wrapped = f"(() => {{ {exec_out} }})()"
            result = ctx.browser.page.evaluate(wrapped)
            return {"snippet": exec_out, "result": result}
        except Exception as e:
            return {"snippet": exec_out, "error": str(e)}
    return {"snippet": exec_out, "result": None, "stub": True}


def _h_extract(_params, exec_out, _ctx):
    if exec_out is None:
        return {"error": "executor não devolveu JSON"}
    try:
        return json.loads(exec_out)
    except json.JSONDecodeError:
        return {"raw": exec_out, "error": "executor JSON inválido"}


def _h_wait(params, _exec_out, ctx):
    sel = params["selector"]
    timeout = params.get("timeout_ms", 5000)
    if ctx.browser._ensure():
        try:
            ctx.browser.page.wait_for_selector(sel, timeout=timeout)
            return {"ok": True}
        except Exception as e:
            return {"ok": False, "error": str(e)}
    return {"ok": True, "stub": True}


def _h_store(params, _exec_out, ctx):
    ctx.vars[params["key"]] = params["value"]
    return {"ok": True}


def _h_respond_user(params, exec_out, _ctx):
    message = exec_out if exec_out else params["message"]
    return {"ok": True, "message": message}


def _h_step_done(params, _exec_out, _ctx):
    return {"__step_done__": True, "evidence": params.get("evidence")}


def _h_finish(params, _exec_out, _ctx):
    return {"__finish__": True, "summary": params.get("summary", "")}


def _h_screenshot(_params, _exec_out, ctx):
    b64 = ctx.browser.screenshot(CONFIG.screenshot_max_width)
    if b64 is None:
        return {"ok": False, "error": "playwright indisponível ou browser não inicializado"}
    return {"ok": True, "format": "png", "bytes": len(b64)}


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
    "screenshot": _h_screenshot,
}

EXECUTOR_TOOLS = {"eval_js", "extract", "respond_user"}
BROWSER_TOOLS = {"navigate", "click", "type", "eval_js", "wait"}


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

def _exec_one_call(call, ctx, step, step_history, global_history, iteration):
    """Run a single call; return (events_to_emit, terminal_or_none)."""
    tool = call["tool"]
    params = _resolve_refs(call.get("params", {}), ctx.vars)
    params = _resolve_credentials(params, ctx.creds)

    exec_out = None
    if tool in EXECUTOR_TOOLS:
        exec_out = call_executor(tool, params, step)

    handler = TOOL_HANDLERS.get(tool)
    if handler is None:
        result = {"error": f"tool desconhecida: {tool}"}
    else:
        result = handler(params, exec_out, ctx)

    save_as = call.get("save_as")
    if save_as:
        ctx.vars[save_as] = result

    safe_params = _redact_credentials(params, ctx.creds)
    entry = {
        "iter": iteration,
        "id": call["id"],
        "tool": tool,
        "params": safe_params,
        "result": result,
        "executor_output": exec_out,
    }
    step_history.append(entry)
    global_history.append(entry)
    events = [("tool_call", {"step_id": step["id"], **entry})]

    if tool in BROWSER_TOOLS and ctx.browser.available:
        b64 = ctx.browser.screenshot(CONFIG.screenshot_max_width)
        if b64:
            events.append(("screenshot", {
                "step_id": step["id"],
                "after_tool": tool,
                "format": "png",
                "data_url": f"data:image/png;base64,{b64}",
                "url": ctx.browser_state.get("url"),
            }))

    terminal = None
    if isinstance(result, dict):
        if result.get("__finish__"):
            terminal = "finish"
        elif result.get("__step_done__"):
            terminal = "step_done"
    return events, terminal


def _redact_credentials(value, creds: dict):
    if not creds:
        return value
    if isinstance(value, str):
        out = value
        for name, secret in creds.items():
            if secret and isinstance(secret, str) and secret in out:
                out = out.replace(secret, f"<credencial:{name}>")
        return out
    if isinstance(value, dict):
        return {k: _redact_credentials(v, creds) for k, v in value.items()}
    if isinstance(value, list):
        return [_redact_credentials(v, creds) for v in value]
    return value


def _execute_calls(calls, ctx, step, step_history, global_history, iteration):
    done = set()
    remaining = list(calls)
    events = []
    terminal = None
    while remaining:
        progressed = False
        for call in list(remaining):
            if not all(dep in done for dep in call.get("depends_on", [])):
                continue
            new_events, t = _exec_one_call(call, ctx, step, step_history, global_history, iteration)
            events.extend(new_events)
            if t:
                terminal = t
                break
            done.add(call["id"])
            remaining.remove(call)
            progressed = True
        if terminal:
            break
        if not progressed:
            raise RuntimeError(f"ciclo em depends_on: {[c['id'] for c in remaining]}")
    return events, terminal


def run_events(task, run_id=None):
    run_id = run_id or uuid.uuid4().hex[:12]
    browser_state = {"url": "about:blank"}
    vars_dict = {}
    global_history = []
    browser = _Browser()
    creds = load_credentials()
    ctx = _Ctx(vars_dict, browser_state, browser, creds)

    yield ("run_start", {
        "run_id": run_id,
        "task": task,
        "config": CONFIG.to_dict(),
        "credentials_known": list(creds.keys()),
    })

    try:
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
            for it in range(CONFIG.max_step_iters):
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
                    events, terminal = _execute_calls(calls, ctx, step, step_history, global_history, it)
                except Exception as e:
                    yield ("error", {"message": str(e), "where": f"exec/{step['id']}"})
                    return
                for ev in events:
                    yield ev
                if terminal:
                    break

            if terminal == "finish":
                yield ("vars_snapshot", {"vars": vars_dict})
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
                if replans > CONFIG.max_replans:
                    yield ("abort", {"reason": f"replan limit ({CONFIG.max_replans}) excedido"})
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

        yield ("vars_snapshot", {"vars": vars_dict})
        yield ("done", {"reason": "plano concluído"})
    finally:
        browser.close()


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
        elif name == "screenshot":
            print(f"  [screenshot] after {payload['after_tool']} ({payload.get('url', '?')})")
        elif name == "critic":
            mark = "ok" if payload["succeeded"] else "fail"
            print(f"[critic/{mark}] {payload['step_id']} action={payload['next_action']} — {payload['reason']}")
        elif name == "router_idle":
            print(f"[router] idle: {payload['reason']}")
        elif name == "run_start":
            print(f"[run {payload['run_id']}] task: {payload['task']}")
        elif name == "vars_snapshot":
            print(f"[vars] {len(payload['vars'])} key(s) stored")
        elif name in ("done", "abort", "error"):
            print(f"[{name}] {payload}")


# --- SSE server ------------------------------------------------------------

CORS_HEADERS = {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
}


class _AgentHandler(BaseHTTPRequestHandler):
    def _set_cors(self):
        for k, v in CORS_HEADERS.items():
            self.send_header(k, v)

    def _write_json(self, status, obj):
        body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self._set_cors()
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_json_body(self):
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length else b"{}"
        return json.loads(raw.decode("utf-8")) if raw else {}

    def do_OPTIONS(self):
        self.send_response(204)
        self._set_cors()
        self.end_headers()

    def do_GET(self):
        if self.path == "/":
            self._write_json(200, {
                "service": "bridge.py",
                "endpoints": ["POST /run", "GET/POST /config", "GET/POST/DELETE /credentials"],
                "playwright": _probe_playwright(),
            })
        elif self.path == "/config":
            self._write_json(200, CONFIG.to_dict())
        elif self.path == "/credentials":
            self._write_json(200, {"known": list(load_credentials().keys())})
        else:
            self._write_json(404, {"error": "not found"})

    def do_POST(self):
        if self.path == "/run":
            self._sse_run()
        elif self.path == "/config":
            try:
                patch = self._read_json_body()
            except json.JSONDecodeError:
                self._write_json(400, {"error": "JSON inválido"})
                return
            with _CONFIG_LOCK:
                CONFIG.update(patch)
            self._write_json(200, CONFIG.to_dict())
        elif self.path == "/credentials":
            try:
                body = self._read_json_body()
            except json.JSONDecodeError:
                self._write_json(400, {"error": "JSON inválido"})
                return
            creds = load_credentials()
            creds.update({k: v for k, v in body.items() if isinstance(k, str)})
            save_credentials(creds)
            self._write_json(200, {"known": list(creds.keys())})
        else:
            self._write_json(404, {"error": "not found"})

    def do_DELETE(self):
        if self.path.startswith("/credentials/"):
            name = self.path[len("/credentials/"):]
            creds = load_credentials()
            creds.pop(name, None)
            save_credentials(creds)
            self._write_json(200, {"known": list(creds.keys())})
        else:
            self._write_json(404, {"error": "not found"})

    def _sse_run(self):
        try:
            body = self._read_json_body()
            task = body["task"]
        except (json.JSONDecodeError, KeyError):
            self._write_json(400, {"error": 'esperado JSON {"task": "..."}'})
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
        sys.stderr.write(f"[server {time.strftime('%H:%M:%S')}] {fmt % args}\n")


def _probe_playwright():
    try:
        import playwright  # noqa: F401
        return {"installed": True}
    except ImportError:
        return {"installed": False, "hint": "pip install playwright && playwright install chromium"}


def serve(host="127.0.0.1", port=8000):
    server = ThreadingHTTPServer((host, port), _AgentHandler)
    pw = _probe_playwright()
    pw_msg = "ON" if pw["installed"] else "OFF (stubs)"
    print(f"bridge SSE em http://{host}:{port}  · playwright: {pw_msg}")
    print(f"credentials file: {CREDENTIALS_PATH}")
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
