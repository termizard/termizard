import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Events } from '@wailsio/runtime'
import '@xterm/xterm/css/xterm.css'
import { TitleBar } from './TitleBar'
import * as svc from './service'

const isMac = navigator.platform.startsWith('Mac') ||
              navigator.userAgent.includes('Macintosh')

function writeB64(term: Terminal, b64: string) {
  term.write(Uint8Array.from(atob(b64), c => c.charCodeAt(0)))
}

export function App() {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef      = useRef<Terminal | null>(null)
  const fitRef       = useRef<FitAddon | null>(null)
  const [title, setTitle]           = useState('termizard')
  const [showTitleBar, setShowTitleBar] = useState(true)

  useEffect(() => {
    const id = setTimeout(() => fitRef.current?.fit(), 60)
    return () => clearTimeout(id)
  }, [showTitleBar])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    let term: Terminal | null = null
    let fitAddon: FitAddon | null = null
    let disposed = false
    let termReady = false
    let offPtyData: (() => void) | null = null
    const pending: string[] = []

    const flushPending = () => {
      if (!term || !termReady) return
      for (const b64 of pending) {
        try { writeB64(term, b64) } catch { /* drop malformed */ }
      }
      pending.length = 0
      term.scrollToBottom()
    }

    offPtyData = Events.On('pty:data', (ev) => {
      const b64 = ev.data as string
      if (typeof b64 !== 'string') return
      if (!termReady || !term) {
        pending.push(b64)
        return
      }
      try { writeB64(term, b64) } catch { /* drop malformed */ }
    })

    async function init() {
      const [cfg, rawTitle] = await Promise.all([
        svc.GetConfig(),
        svc.GetInitialTitle(),
      ])
      setShowTitleBar(cfg.showTitleBar)
      const initTitle = rawTitle || 'termizard'
      setTitle(initTitle)
      svc.SetTitle(initTitle).catch(() => {})

      fitAddon = new FitAddon()
      term = new Terminal({
        fontFamily:            cfg.fontFamily,
        fontSize:              cfg.fontSize,
        cursorStyle:           cfg.cursorStyle,
        cursorBlink:           cfg.cursorBlink,
        theme:                 cfg.theme,
        allowTransparency:     true,
        scrollback:            5000,
        scrollOnUserInput:     true,
        macOptionIsMeta:       true,
        rightClickSelectsWord: false,
      })

      term.loadAddon(fitAddon)
      term.loadAddon(new WebLinksAddon())
      term.open(container!)
      if (disposed) { term.dispose(); return }

      fitRef.current  = fitAddon
      termRef.current = term

      fitAddon.fit()
      termReady = true
      await svc.Ready(term.cols, term.rows)
      flushPending()
      term.scrollToBottom()

      // Finder-launched .app bundles need an extra focus nudge for keyboard input.
      setTimeout(() => term?.focus(), 0)
      setTimeout(() => term?.focus(), 150)

      term.onTitleChange((t: string) => {
        const cleaned = t || 'termizard'
        setTitle(cleaned)
        svc.SetTitle(cleaned).catch(() => {})
      })

      term.onData((data: string) => {
        svc.SendInput(data).catch(() => {})
      })

      term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
        svc.Resize(cols, rows).catch(() => {})
      })

      term.attachCustomKeyEventHandler((e: KeyboardEvent): boolean => {
        if (e.type !== 'keydown') return true

        const meta  = e.metaKey
        const ctrl  = e.ctrlKey
        const shift = e.shiftKey
        const alt   = e.altKey
        const k     = e.key

        if (isMac && meta && !ctrl && !shift && !alt && k === 'c') {
          if (term!.hasSelection()) {
            navigator.clipboard.writeText(term!.getSelection()).catch(() => {})
            return false
          }
          return true
        }

        if (!isMac && ctrl && shift && !meta && !alt && k === 'C') {
          if (term!.hasSelection()) {
            navigator.clipboard.writeText(term!.getSelection()).catch(() => {})
          }
          return false
        }

        if ((isMac  && meta && !ctrl && !shift && !alt && k === 'n') ||
            (!isMac && ctrl && shift && !meta  && !alt && k === 'N')) {
          svc.NewWindow().catch(() => {})
          return false
        }

        return true
      })

      window.addEventListener('paste', onPaste, true)
      container!.addEventListener('contextmenu', onContextMenu)

      term.focus()
    }

    function onPaste(e: ClipboardEvent) {
      e.stopPropagation()
      e.preventDefault()
      const text = e.clipboardData?.getData('text/plain') ?? ''
      if (text) svc.SendInput(text).catch(() => {})
    }

    function onContextMenu(e: MouseEvent) {
      e.preventDefault()
      navigator.clipboard.readText()
        .then(text => { if (text) svc.SendInput(text).catch(() => {}) })
        .catch(() => {})
    }

    init().catch(console.error)

    const onWindowResize = () => fitRef.current?.fit()
    window.addEventListener('resize', onWindowResize)

    return () => {
      disposed = true
      offPtyData?.()
      window.removeEventListener('resize', onWindowResize)
      window.removeEventListener('paste', onPaste, true)
      container?.removeEventListener('contextmenu', onContextMenu)
      term?.dispose()
      termRef.current = null
      fitRef.current  = null
    }
  }, [])

  return (
    <div className="app">
      {showTitleBar && (
        <TitleBar
          title={title}
          onToggleMaximize={() => svc.ToggleMaximize().catch(() => {})}
        />
      )}
      <div className="terminal-container" ref={containerRef} />
    </div>
  )
}
