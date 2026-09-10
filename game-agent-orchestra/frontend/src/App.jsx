import React, { useEffect, useRef, useState } from 'react'
import { marked } from 'marked'

// Os 6 agentes do pipeline, na ordem
const AGENTS = [
  { id: 'maestro', label: '🎼 Maestro', desc: 'Briefing criativo' },
  { id: 'scout', label: '🔍 Scout', desc: 'Queries de busca' },
  { id: 'browser', label: '🌐 Browser', desc: 'Pesquisa web (Playwright)' },
  { id: 'vision', label: '👁 Vision', desc: 'Análise visual (Qwen)' },
  { id: 'architect', label: '📐 Architect', desc: 'Game Design Document' },
  { id: 'judge', label: '⚖️ Judge', desc: 'Crítica e veredicto' },
]

const STATUS_ICON = {
  idle: '·', running: '⏳', streaming: '✍️', warning: '⚠️',
  done: '✅', error: '❌', skipped: '⏭️',
}

export default function App() {
  const [catalog, setCatalog] = useState({ categorias: {}, temas: {} })
  const [categoria, setCategoria] = useState('')
  const [tema, setTema] = useState('')
  const [runId, setRunId] = useState(null)
  const [runState, setRunState] = useState('idle') // idle | running | done | error
  const [agentStatus, setAgentStatus] = useState({})
  const [agentDetail, setAgentDetail] = useState({})
  const [thinking, setThinking] = useState({})   // agent -> texto de raciocínio
  const [output, setOutput] = useState({})       // agent -> output final
  const [activeAgent, setActiveAgent] = useState(null)
  const [tab, setTab] = useState('pipeline')     // pipeline | gdd | recursos | historico
  const [resources, setResources] = useState(null)
  const [runs, setRuns] = useState([])
  const esRef = useRef(null)
  const thinkRef = useRef(null)
  const outRef = useRef(null)

  // Carrega catálogo de categorias/temas na inicialização
  useEffect(() => {
    fetch('/api/catalog').then(r => r.json()).then(data => {
      setCatalog(data)
      setCategoria(Object.keys(data.categorias)[0] || '')
      setTema(Object.keys(data.temas)[0] || '')
    }).catch(() => {})
  }, [])

  // Auto-scroll das colunas de thinking/output
  useEffect(() => {
    if (thinkRef.current) thinkRef.current.scrollTop = thinkRef.current.scrollHeight
    if (outRef.current) outRef.current.scrollTop = outRef.current.scrollHeight
  }, [thinking, output, activeAgent])

  const loadRuns = () =>
    fetch('/api/runs').then(r => r.json()).then(setRuns).catch(() => {})

  const loadResources = (id) =>
    fetch(`/api/resources/${id}`).then(r => r.json()).then(setResources).catch(() => {})

  // Inicia uma nova run e conecta no SSE
  const startRun = async () => {
    if (runState === 'running') return
    setAgentStatus({}); setAgentDetail({}); setThinking({}); setOutput({})
    setResources(null); setTab('pipeline'); setActiveAgent('maestro')
    setRunState('running')

    const resp = await fetch('/api/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ categoria, tema }),
    })
    if (!resp.ok) { setRunState('error'); return }
    const { run_id } = await resp.json()
    setRunId(run_id)

    const es = new EventSource(`/api/run/stream?run_id=${run_id}`)
    esRef.current = es
    es.onmessage = (msg) => {
      const ev = JSON.parse(msg.data)
      const a = ev.agent
      if (ev.status === 'end') { es.close(); return }
      if (a === 'orchestrator') {
        if (ev.status === 'done') { setRunState('done'); loadResources(run_id); loadRuns() }
        if (ev.status === 'error') { setRunState('error') }
        return
      }
      setActiveAgent(prev => (ev.status === 'running' || ev.status === 'streaming') ? a : prev)
      setAgentStatus(s => {
        // não regride done -> streaming por eventos fora de ordem
        if (s[a] === 'done' && ev.status === 'streaming') return s
        return { ...s, [a]: ev.status }
      })
      if (ev.detail) setAgentDetail(d => ({ ...d, [a]: ev.detail }))
      if (ev.thinking) setThinking(t => ({ ...t, [a]: (t[a] || '') + ev.thinking }))
      if (ev.token) setOutput(o => ({ ...o, [a]: (o[a] || '') + ev.token }))
    }
    es.onerror = () => { es.close() }
  }

  useEffect(() => { loadRuns() }, [])

  // Carrega uma run antiga do histórico
  const openRun = async (id) => {
    const run = await fetch(`/api/run/${id}`).then(r => r.json())
    setRunId(id)
    setRunState(run.status)
    setOutput({
      maestro: run.briefing || '',
      scout: run.queries ? JSON.stringify(run.queries, null, 2) : '',
      vision: run.vision || '',
      architect: run.gdd || '',
      judge: run.critique || '',
    })
    setThinking({})
    setAgentStatus(Object.fromEntries(AGENTS.map(a => [a.id, 'done'])))
    loadResources(id)
    setTab(run.gdd ? 'gdd' : 'pipeline')
  }

  const gdd = output.architect || ''
  const critique = output.judge || ''
  const shown = activeAgent || 'maestro'

  return (
    <div className="app">
      <header>
        <h1>🎮 Game Agent Orchestra</h1>
        <p className="sub">6 agentes locais projetando games onde o ML é o protagonista visível</p>
      </header>

      {/* ── Controles ── */}
      <section className="controls">
        <label>
          Categoria de ML
          <select value={categoria} onChange={e => setCategoria(e.target.value)}>
            {Object.keys(catalog.categorias).map(c => <option key={c}>{c}</option>)}
          </select>
        </label>
        <label>
          Tema visual
          <select value={tema} onChange={e => setTema(e.target.value)}>
            {Object.keys(catalog.temas).map(t => <option key={t}>{t}</option>)}
          </select>
        </label>
        <button onClick={startRun} disabled={runState === 'running' || !categoria}>
          {runState === 'running' ? 'Orquestrando…' : '▶ Iniciar Pipeline'}
        </button>
        {runId && <span className="runid">run: {runId} · {runState}</span>}
      </section>

      {/* ── Painel dos 6 agentes ── */}
      <section className="agents">
        {AGENTS.map(a => {
          const st = agentStatus[a.id] || 'idle'
          return (
            <div
              key={a.id}
              className={`agent ${st} ${shown === a.id ? 'active' : ''}`}
              onClick={() => setActiveAgent(a.id)}
            >
              <div className="agent-head">
                <span>{a.label}</span>
                <span className="icon">{STATUS_ICON[st] || '·'}</span>
              </div>
              <div className="agent-desc">{agentDetail[a.id] || a.desc}</div>
            </div>
          )
        })}
      </section>

      {/* ── Abas ── */}
      <nav className="tabs">
        {['pipeline', 'gdd', 'recursos', 'historico'].map(t => (
          <button key={t} className={tab === t ? 'on' : ''} onClick={() => { setTab(t); if (t === 'historico') loadRuns(); if (t === 'recursos' && runId) loadResources(runId) }}>
            {{ pipeline: '🧠 Ao Vivo', gdd: '📄 Game Design', recursos: '🖼 Recursos', historico: '🕘 Histórico' }[t]}
          </button>
        ))}
      </nav>

      {/* ── Aba: pipeline ao vivo (thinking + output) ── */}
      {tab === 'pipeline' && (
        <section className="live">
          <div className="col">
            <h3>💭 Thinking — {shown}</h3>
            <pre ref={thinkRef} className="stream thinking">{thinking[shown] || '(sem raciocínio ainda)'}</pre>
          </div>
          <div className="col">
            <h3>📝 Output — {shown}</h3>
            <pre ref={outRef} className="stream">{output[shown] || '(sem output ainda)'}</pre>
          </div>
        </section>
      )}

      {/* ── Aba: GDD renderizado + crítica ── */}
      {tab === 'gdd' && (
        <section className="gdd">
          {gdd
            ? <article dangerouslySetInnerHTML={{ __html: marked.parse(gdd) }} />
            : <p className="empty">O Architect ainda não produziu o GDD.</p>}
          {critique && (
            <details open className="critique">
              <summary>⚖️ Crítica do Judge</summary>
              <article dangerouslySetInnerHTML={{ __html: marked.parse(critique) }} />
            </details>
          )}
        </section>
      )}

      {/* ── Aba: recursos coletados ── */}
      {tab === 'recursos' && (
        <section className="resources">
          {!resources && <p className="empty">Sem recursos ainda.</p>}
          {resources && (
            <>
              <h3>Imagens baixadas ({resources.images.length})</h3>
              <div className="grid">
                {resources.images.map(src => (
                  <a key={src} href={src} target="_blank" rel="noreferrer">
                    <img src={src} alt="" loading="lazy" />
                  </a>
                ))}
              </div>
              <h3>Screenshots das páginas ({resources.screenshots.length})</h3>
              <div className="grid">
                {resources.screenshots.map(src => (
                  <a key={src} href={src} target="_blank" rel="noreferrer">
                    <img src={src} alt="" loading="lazy" />
                  </a>
                ))}
              </div>
              <h3>Páginas visitadas</h3>
              <ul>
                {resources.pages.map((p, i) => (
                  <li key={i}>
                    <a href={p.url} target="_blank" rel="noreferrer">{p.title || p.url}</a>
                    {p.query && <em> — “{p.query}”</em>}
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      )}

      {/* ── Aba: histórico ── */}
      {tab === 'historico' && (
        <section className="history">
          <table>
            <thead><tr><th>Run</th><th>Categoria</th><th>Tema</th><th>Status</th><th>Criada</th></tr></thead>
            <tbody>
              {runs.map(r => (
                <tr key={r.id} onClick={() => openRun(r.id)}>
                  <td className="mono">{r.id}</td>
                  <td>{r.categoria}</td>
                  <td>{r.tema}</td>
                  <td>{r.status}</td>
                  <td>{new Date(r.created_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </div>
  )
}
