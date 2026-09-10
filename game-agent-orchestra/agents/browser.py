"""
agents/browser.py — BROWSER, agent-browser com Playwright.

Para cada query do Scout:
  1. Busca no DuckDuckGo (HTML) — ou SerpAPI se houver SERPAPI_KEY no ambiente.
  2. Abre as melhores páginas com Playwright headless (respeitando robots.txt).
  3. Extrai texto, links e URLs de imagens; tira screenshot.
  4. Salva tudo em resources/{run_id}/: HTML, screenshots, imagens baixadas
     e um metadata.json com o sumário estruturado.

Se o Playwright não conseguir iniciar (ex.: chromium não instalado), cai
para um fallback via httpx + BeautifulSoup — o pipeline nunca quebra aqui.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import re
import urllib.parse
import urllib.robotparser
from pathlib import Path

import httpx

from agents.base import BaseAgent

USER_AGENT = (
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/126.0 Safari/537.36 game-agent-orchestra/1.0"
)

# Extensões de imagem que valem a pena baixar
_IMG_EXT = re.compile(r"\.(?:png|jpe?g|webp|gif)(?:\?|$)", re.IGNORECASE)


class BrowserAgent(BaseAgent):
    name = "browser"

    def __init__(self, *args, **kwargs) -> None:
        super().__init__(*args, **kwargs)
        self._robots_cache: dict[str, urllib.robotparser.RobotFileParser | None] = {}

    # ══════════════════ Pipeline principal ══════════════════

    async def run(self, queries: list[str], run_id: str) -> dict:
        await self.status("running", f"Pesquisando {len(queries)} queries na web")

        res_dir = Path(self.config.get("resources_dir", "resources")) / run_id
        (res_dir / "html").mkdir(parents=True, exist_ok=True)
        (res_dir / "images").mkdir(parents=True, exist_ok=True)
        (res_dir / "screenshots").mkdir(parents=True, exist_ok=True)

        max_queries = int(self.config.get("max_queries_browser", 5))
        max_pages = int(self.config.get("max_pages_per_query", 3))

        research: dict = {"queries": [], "pages": [], "images": []}

        playwright = browser = None
        try:
            # Tenta subir o Playwright; se falhar, seguimos só com httpx
            try:
                from playwright.async_api import async_playwright

                playwright = await async_playwright().start()
                browser = await playwright.chromium.launch(
                    headless=bool(self.config.get("browser_headless", True))
                )
            except Exception as exc:  # noqa: BLE001 — fallback deliberado
                await self.status(
                    "warning", f"Playwright indisponível ({exc}); usando fallback httpx"
                )

            for query in queries[:max_queries]:
                await self.status("running", f"Buscando: {query}")
                try:
                    results = await self._search(query)
                except Exception as exc:  # noqa: BLE001
                    await self.status("warning", f"Busca falhou para '{query}': {exc}")
                    continue

                research["queries"].append({
                    "query": query,
                    "results": [r["url"] for r in results[:max_pages]],
                })

                for result in results[:max_pages]:
                    url = result["url"]
                    if not self._robots_allowed(url):
                        continue
                    try:
                        page_data = await self._visit(browser, url, res_dir)
                        if page_data:
                            page_data["query"] = query
                            research["pages"].append(page_data)
                    except Exception as exc:  # noqa: BLE001
                        await self.status("warning", f"Falha em {url}: {exc}")

            # Baixa as imagens mais promissoras encontradas nas páginas
            research["images"] = await self._download_images(research["pages"], res_dir)

        finally:
            if browser:
                await browser.close()
            if playwright:
                await playwright.stop()

        # Persiste o sumário estruturado da pesquisa
        (res_dir / "metadata.json").write_text(
            json.dumps(research, ensure_ascii=False, indent=2), encoding="utf-8"
        )

        await self.status(
            "done",
            f"{len(research['pages'])} páginas, {len(research['images'])} imagens salvas",
        )
        return research

    # ══════════════════ Busca ══════════════════

    async def _search(self, query: str) -> list[dict]:
        """SerpAPI se houver key; senão DuckDuckGo HTML."""
        serp_key = os.environ.get("SERPAPI_KEY")
        if serp_key:
            return await self._search_serpapi(query, serp_key)
        return await self._search_duckduckgo(query)

    async def _search_serpapi(self, query: str, key: str) -> list[dict]:
        async with httpx.AsyncClient(timeout=20) as client:
            resp = await client.get(
                "https://serpapi.com/search.json",
                params={"q": query, "api_key": key, "num": 8},
            )
            resp.raise_for_status()
            data = resp.json()
        return [
            {"url": item["link"], "title": item.get("title", "")}
            for item in data.get("organic_results", [])
            if item.get("link")
        ]

    async def _search_duckduckgo(self, query: str) -> list[dict]:
        """Busca via DuckDuckGo HTML (sem JS, fácil de parsear)."""
        from bs4 import BeautifulSoup

        async with httpx.AsyncClient(
            timeout=20, headers={"User-Agent": USER_AGENT}, follow_redirects=True
        ) as client:
            resp = await client.post(
                "https://html.duckduckgo.com/html/", data={"q": query}
            )
            resp.raise_for_status()

        soup = BeautifulSoup(resp.text, "html.parser")
        results: list[dict] = []
        for anchor in soup.select("a.result__a"):
            href = anchor.get("href", "")
            # DDG embrulha o link real em /l/?uddg=<url-encoded>
            if "uddg=" in href:
                parsed = urllib.parse.parse_qs(urllib.parse.urlparse(href).query)
                href = parsed.get("uddg", [href])[0]
            if href.startswith("http"):
                results.append({"url": href, "title": anchor.get_text(strip=True)})
        return results

    # ══════════════════ robots.txt básico ══════════════════

    def _robots_allowed(self, url: str) -> bool:
        """Checagem básica de robots.txt (cacheada por host)."""
        parsed = urllib.parse.urlparse(url)
        base = f"{parsed.scheme}://{parsed.netloc}"
        if base not in self._robots_cache:
            parser = urllib.robotparser.RobotFileParser()
            try:
                resp = httpx.get(
                    f"{base}/robots.txt",
                    timeout=8,
                    headers={"User-Agent": USER_AGENT},
                    follow_redirects=True,
                )
                if resp.status_code == 200:
                    parser.parse(resp.text.splitlines())
                    self._robots_cache[base] = parser
                else:
                    self._robots_cache[base] = None  # sem robots.txt → permitido
            except Exception:  # noqa: BLE001
                self._robots_cache[base] = None
        parser = self._robots_cache[base]
        if parser is None:
            return True
        return parser.can_fetch(USER_AGENT, url)

    # ══════════════════ Visita de página ══════════════════

    async def _visit(self, browser, url: str, res_dir: Path) -> dict | None:
        """Abre a página (Playwright se disponível) e extrai/salva conteúdo."""
        slug = hashlib.sha1(url.encode()).hexdigest()[:10]

        if browser is not None:
            page = await browser.new_page(user_agent=USER_AGENT)
            try:
                await page.goto(url, timeout=25_000, wait_until="domcontentloaded")
                await page.wait_for_timeout(1200)  # espera renderizar um pouco
                title = await page.title()
                html = await page.content()
                text = await page.evaluate("() => document.body?.innerText || ''")
                img_srcs = await page.evaluate(
                    "() => Array.from(document.images).map(i => i.src)"
                )
                links = await page.evaluate(
                    "() => Array.from(document.links).slice(0, 30).map(a => a.href)"
                )
                await page.screenshot(
                    path=str(res_dir / "screenshots" / f"{slug}.png"), full_page=False
                )
            finally:
                await page.close()
        else:
            # Fallback sem browser: httpx + BeautifulSoup
            from bs4 import BeautifulSoup

            async with httpx.AsyncClient(
                timeout=20, headers={"User-Agent": USER_AGENT}, follow_redirects=True
            ) as client:
                resp = await client.get(url)
                resp.raise_for_status()
                html = resp.text
            soup = BeautifulSoup(html, "html.parser")
            title = soup.title.get_text(strip=True) if soup.title else url
            text = soup.get_text(" ", strip=True)
            img_srcs = [
                urllib.parse.urljoin(url, img.get("src", ""))
                for img in soup.find_all("img")
            ]
            links = [
                urllib.parse.urljoin(url, a.get("href", ""))
                for a in soup.find_all("a")[:30]
            ]

        # Salva o HTML bruto
        (res_dir / "html" / f"{slug}.html").write_text(html, encoding="utf-8")

        # Filtra imagens plausíveis (URLs absolutas com extensão de imagem)
        images = [
            src for src in img_srcs
            if src.startswith("http") and _IMG_EXT.search(src)
        ][: int(self.config.get("max_images_per_page", 4))]

        return {
            "url": url,
            "title": title,
            "slug": slug,
            "text_excerpt": (text or "")[:2500],
            "links": [l for l in links if l.startswith("http")][:15],
            "image_urls": images,
        }

    # ══════════════════ Download de imagens ══════════════════

    async def _download_images(self, pages: list[dict], res_dir: Path) -> list[dict]:
        """Baixa imagens candidatas (até um limite global) para resources/."""
        saved: list[dict] = []
        limit = int(self.config.get("max_images_vision", 6)) * 2  # margem para o Vision

        async with httpx.AsyncClient(
            timeout=15, headers={"User-Agent": USER_AGENT}, follow_redirects=True
        ) as client:
            for page in pages:
                for img_url in page.get("image_urls", []):
                    if len(saved) >= limit:
                        return saved
                    try:
                        resp = await client.get(img_url)
                        content_type = resp.headers.get("content-type", "")
                        # Ignora ícones minúsculos e não-imagens
                        if resp.status_code != 200 or len(resp.content) < 8_000:
                            continue
                        if not content_type.startswith("image/"):
                            continue
                        ext = content_type.split("/")[-1].split(";")[0]
                        ext = {"jpeg": "jpg"}.get(ext, ext)[:4] or "img"
                        name = hashlib.sha1(img_url.encode()).hexdigest()[:12]
                        filename = f"{name}.{ext}"
                        (res_dir / "images" / filename).write_bytes(resp.content)
                        saved.append({
                            "file": filename,
                            "url": img_url,
                            "source_page": page["url"],
                            "bytes": len(resp.content),
                        })
                    except Exception:  # noqa: BLE001 — imagem individual não importa
                        continue
                    await asyncio.sleep(0.2)  # gentileza com os servidores
        return saved
