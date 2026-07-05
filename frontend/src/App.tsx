import { useCallback, useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Events } from '@wailsio/runtime'
import '@xterm/xterm/css/xterm.css'
import { reorderTabsAt } from './tabs'
import { TitleBar } from './TitleBar'
import { TabBar } from './TabBar'
import * as svc from './service'
import {
  buildBindingList,
  handleTerminalAction,
  matchBinding,
  type ConfigKeybinding,
} from './keybindings'

const isMac = navigator.platform.startsWith('Mac') ||
              navigator.userAgent.includes('Macintosh')

interface PtyPayload {
  tabId: number
  data: string
}

interface TabRuntime {
  id: number
  pathTitle: string
  term: Terminal | null
  fitAddon: FitAddon | null
  termReady: boolean
  pending: string[]
}

interface TabOps {
  addTab: () => Promise<void>
  closeTab: (tabId: number, fromPTY?: boolean) => Promise<void>
  nextTab: () => void
  prevTab: () => void
  moveTab: (delta: number) => void
}

function writeB64(term: Terminal, b64: string) {
  term.write(Uint8Array.from(atob(b64), c => c.charCodeAt(0)))
}

function parsePtyPayload(raw: unknown): PtyPayload | null {
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw) as PtyPayload
      if (typeof parsed.tabId === 'number' && typeof parsed.data === 'string') {
        return parsed
      }
    } catch {
      return { tabId: 0, data: raw }
    }
  }
  if (raw && typeof raw === 'object') {
    const obj = raw as Record<string, unknown>
    if (typeof obj.tabId === 'number' && typeof obj.data === 'string') {
      return { tabId: obj.tabId, data: obj.data }
    }
  }
  return null
}

function displayLabel(
  tab: TabRuntime,
  index: number,
  mode: svc.TabLabelMode,
  initTitle: string,
): string {
  if (mode === 'index') {
    return `Tab ${index + 1}`
  }
  return tab.pathTitle || initTitle || '~'
}

function buildTabList(
  order: number[],
  tabs: Map<number, TabRuntime>,
  mode: svc.TabLabelMode,
  initTitle: string,
): { id: number; title: string }[] {
  return order
    .filter((id) => tabs.has(id))
    .map((id, index) => ({
      id,
      title: displayLabel(tabs.get(id)!, index, mode, initTitle),
    }))
}

function attachInputHandlers(
  tab: TabRuntime,
  tabId: number,
  tabsEnabled: boolean,
  ops: TabOps,
  onActiveTitle: (t: string) => void,
  keybindings: ConfigKeybinding[],
  baseFontSize: number,
  onFontSize: (size: number) => void,
) {
  const term = tab.term!
  const bindings = buildBindingList(keybindings)

  term.onTitleChange((t: string) => {
    tab.pathTitle = t || tab.pathTitle
    onActiveTitle(tab.pathTitle)
  })

  term.onData((data: string) => {
    svc.SendInput(tabId, data).catch(() => {})
  })

  term.onResize(({ cols, rows }: { cols: number; rows: number }) => {
    svc.Resize(tabId, cols, rows).catch(() => {})
  })

  term.attachCustomKeyEventHandler((e: KeyboardEvent): boolean => {
    if (e.type !== 'keydown') return true

    const action = matchBinding(e, bindings)
    if (action) {
      return handleTerminalAction(action, {
        term,
        sendInput: (data) => { svc.SendInput(tabId, data).catch(() => {}) },
        baseFontSize,
        onFontSize: (size) => {
          onFontSize(size)
          tab.fitAddon?.fit()
        },
      })
    }

    const meta  = e.metaKey
    const ctrl  = e.ctrlKey
    const shift = e.shiftKey
    const alt   = e.altKey
    const k     = e.key

    if (tabsEnabled) {
      if ((isMac && meta && !ctrl && !shift && !alt && k === 't') ||
          (!isMac && ctrl && shift && !meta && !alt && k === 'T')) {
        ops.addTab().catch(() => {})
        return false
      }
      if ((isMac && meta && !ctrl && !shift && !alt && k === 'w') ||
          (!isMac && ctrl && shift && !meta && !alt && k === 'W')) {
        ops.closeTab(tabId).catch(() => {})
        return false
      }
      if (isMac && meta && shift && !ctrl && !alt && (k === ']' || k === '}')) {
        ops.nextTab()
        return false
      }
      if (isMac && meta && shift && !ctrl && !alt && (k === '[' || k === '{')) {
        ops.prevTab()
        return false
      }
      if (isMac && meta && shift && !ctrl && !alt && k === '}') {
        ops.moveTab(1)
        return false
      }
      if (isMac && meta && shift && !ctrl && !alt && k === '{') {
        ops.moveTab(-1)
        return false
      }
      if (!isMac && ctrl && !shift && !alt && k === 'Tab') {
        if (meta) ops.prevTab()
        else ops.nextTab()
        return false
      }
    } else if ((isMac && meta && !ctrl && !shift && !alt && k === 'n') ||
               (!isMac && ctrl && shift && !meta && !alt && k === 'N')) {
      svc.NewWindow().catch(() => {})
      return false
    }

    return true
  })
}

export function App() {
  const stackRef   = useRef<HTMLDivElement>(null)
  const tabsRef    = useRef<Map<number, TabRuntime>>(new Map())
  const orderRef   = useRef<number[]>([0])
  const activeRef  = useRef(0)
  const opsRef     = useRef<TabOps | null>(null)
  const labelModeRef = useRef<svc.TabLabelMode>('path')
  const initTitleRef = useRef('~')
  const keybindingsRef = useRef<ConfigKeybinding[]>([])
  const baseFontSizeRef = useRef(13)

  const [title, setTitle]                 = useState('termizard')
  const [showTitleBar, setShowTitleBar]   = useState(true)
  const [titlePosition, setTitlePosition] = useState<'left' | 'center' | 'right'>('center')
  const [tabsEnabled, setTabsEnabled]     = useState(false)
  const [showNewButton, setShowNewButton] = useState(true)
  const [showWhenSingle, setShowWhenSingle] = useState(false)
  const [tabList, setTabList]             = useState<{ id: number; title: string }[]>([])
  const [activeTab, setActiveTab]         = useState(0)
  const [chromeReady, setChromeReady]     = useState(false)

  // CSS animations pause when the webview loses focus; drive the backdrop with rAF instead.
  useEffect(() => {
    const root = document.documentElement
    let raf = 0
    const period = 20000
    const start = performance.now()

    const animate = (now: number) => {
      const t = ((now - start) % period) / period
      let x = 0
      let y = 50
      if (t < 1 / 3) {
        const p = t / (1 / 3)
        x = p * 100
        y = 50 - p * 50
      } else if (t < 2 / 3) {
        const p = (t - 1 / 3) / (1 / 3)
        x = 100 - p * 50
        y = p * 100
      } else {
        const p = (t - 2 / 3) / (1 / 3)
        x = 50 - p * 50
        y = 100 - p * 50
      }
      root.style.setProperty('--jb-shimmer-x', `${x}%`)
      root.style.setProperty('--jb-shimmer-y', `${y}%`)
      raf = requestAnimationFrame(animate)
    }

    raf = requestAnimationFrame(animate)
    return () => cancelAnimationFrame(raf)
  }, [])

  const onFontSize = useCallback((size: number) => {
    document.documentElement.style.setProperty('--terminal-font-size', `${size}px`)
  }, [])

  const syncTabList = useCallback(() => {
    setTabList(buildTabList(orderRef.current, tabsRef.current, labelModeRef.current, initTitleRef.current))
  }, [])

  const activateTab = useCallback((tabId: number) => {
    activeRef.current = tabId
    setActiveTab(tabId)

    const stack = stackRef.current
    if (stack) {
      stack.querySelectorAll('.terminal-pane').forEach((el) => {
        const pane = el as HTMLElement
        pane.classList.toggle('terminal-pane--active', pane.dataset.tabId === String(tabId))
      })
    }

    const tab = tabsRef.current.get(tabId)
    if (tab) {
      const idx = orderRef.current.indexOf(tabId)
      const label = displayLabel(tab, idx >= 0 ? idx : 0, labelModeRef.current, initTitleRef.current)
      setTitle(label)
      svc.SetTitle(label).catch(() => {})
      setTimeout(() => {
        tab.fitAddon?.fit()
        tab.term?.focus()
      }, 0)
    }
  }, [])

  const fitActive = useCallback(() => {
    tabsRef.current.get(activeRef.current)?.fitAddon?.fit()
  }, [])

  useEffect(() => {
    const id = setTimeout(() => fitActive(), 60)
    return () => clearTimeout(id)
  }, [showTitleBar, tabsEnabled, tabList.length, activeTab, chromeReady, fitActive])

  useEffect(() => {
    const stack = stackRef.current
    if (!stack) return

    let disposed = false
    let offPtyData: (() => void) | null = null
    let offTabClosed: (() => void) | null = null

    const flushPending = (tab: TabRuntime) => {
      if (!tab.term || !tab.termReady) return
      for (const b64 of tab.pending) {
        try { writeB64(tab.term, b64) } catch { /* drop malformed */ }
      }
      tab.pending.length = 0
      tab.term.scrollToBottom()
    }

    offPtyData = Events.On('pty:data', (ev) => {
      const payload = parsePtyPayload(ev.data)
      if (!payload) return
      const tab = tabsRef.current.get(payload.tabId)
      if (!tab) return
      if (!tab.termReady || !tab.term) {
        tab.pending.push(payload.data)
        return
      }
      try {
        writeB64(tab.term, payload.data)
        tab.term.scrollToBottom()
      } catch { /* drop malformed */ }
    })

    offTabClosed = Events.On('tab:closed', (ev) => {
      let tabId = 0
      if (typeof ev.data === 'string') {
        try {
          tabId = (JSON.parse(ev.data) as { tabId: number }).tabId
        } catch { return }
      } else if (ev.data && typeof ev.data === 'object') {
        tabId = (ev.data as { tabId: number }).tabId
      }
      opsRef.current?.closeTab(tabId, true).catch(() => {})
    })

    function onPaste(e: ClipboardEvent) {
      e.stopPropagation()
      e.preventDefault()
      const text = e.clipboardData?.getData('text/plain') ?? ''
      if (text) svc.SendInput(activeRef.current, text).catch(() => {})
    }

    function onContextMenu(e: MouseEvent) {
      e.preventDefault()
      navigator.clipboard.readText()
        .then(text => { if (text) svc.SendInput(activeRef.current, text).catch(() => {}) })
        .catch(() => {})
    }

    async function mountTab(
      tabId: number,
      cfg: svc.XTermConfig,
      initTitle: string,
      isActive: boolean,
      multiTab: boolean,
    ) {
      if (!stack) return

      let pane = stack.querySelector(`[data-tab-id="${tabId}"]`) as HTMLDivElement | null
      if (!pane) {
        pane = document.createElement('div')
        pane.className = 'terminal-pane'
        pane.dataset.tabId = String(tabId)
        stack.appendChild(pane)
      }
      if (isActive) {
        pane.classList.add('terminal-pane--active')
      }
      if (cfg.cursorBlink) {
        pane.classList.add('terminal-pane--cursor-blink')
      }

      const inactiveCursor =
        cfg.cursorStyle === 'bar' ? 'bar'
        : cfg.cursorStyle === 'underline' ? 'underline'
        : 'block'

      const fitAddon = new FitAddon()
      const term = new Terminal({
        fontFamily:            cfg.fontFamily,
        fontSize:              cfg.fontSize,
        fontWeightBold:        'bold',
        lineHeight:            1.15,
        cursorStyle:           cfg.cursorStyle,
        cursorBlink:           cfg.cursorBlink,
        cursorInactiveStyle:   inactiveCursor,
        theme:                 cfg.theme,
        allowTransparency:     true,
        scrollback:            5000,
        scrollOnUserInput:     true,
        macOptionIsMeta:       true,
        rightClickSelectsWord: false,
      })

      term.loadAddon(fitAddon)
      term.loadAddon(new WebLinksAddon())
      term.open(pane)
      if (disposed) { term.dispose(); return }

      fitAddon.fit()

      const runtime: TabRuntime = {
        id: tabId,
        pathTitle: initTitle,
        term,
        fitAddon,
        termReady: false,
        pending: [],
      }
      tabsRef.current.set(tabId, runtime)

      const onActiveTitle = (t: string) => {
        runtime.pathTitle = t
        syncTabList()
        if (activeRef.current === tabId) {
          const idx = orderRef.current.indexOf(tabId)
          const label = displayLabel(runtime, idx >= 0 ? idx : 0, labelModeRef.current, initTitleRef.current)
          setTitle(label)
          svc.SetTitle(label).catch(() => {})
        }
      }

      attachInputHandlers(
        runtime,
        tabId,
        multiTab,
        opsRef.current!,
        onActiveTitle,
        keybindingsRef.current,
        baseFontSizeRef.current,
        onFontSize,
      )

      runtime.termReady = true
      await svc.Ready(tabId, term.cols, term.rows)
      flushPending(runtime)
      term.scrollToBottom()

      if (isActive) {
        setTimeout(() => term.focus(), 0)
        setTimeout(() => term.focus(), 150)
      }
    }

    async function removeTabUI(tabId: number, fromPTY: boolean) {
      const idx = orderRef.current.indexOf(tabId)
      if (idx < 0) return

      const tab = tabsRef.current.get(tabId)
      tab?.term?.dispose()
      tabsRef.current.delete(tabId)
      orderRef.current.splice(idx, 1)

      const pane = stack?.querySelector(`[data-tab-id="${tabId}"]`)
      pane?.remove()

      if (orderRef.current.length === 0) return

      let nextId = activeRef.current
      if (activeRef.current === tabId || !tabsRef.current.has(activeRef.current)) {
        const nextIdx = Math.min(idx, orderRef.current.length - 1)
        nextId = orderRef.current[nextIdx]
      }
      syncTabList()
      activateTab(nextId)

      if (!fromPTY) {
        try { await svc.CloseTab(tabId) } catch { /* already closed */ }
      }
    }

    opsRef.current = {
      addTab: async () => {
        const info = await svc.NewTab()
        if (!orderRef.current.includes(info.id)) {
          orderRef.current.push(info.id)
        }
        syncTabList()
        activateTab(info.id)
        const cfg = await svc.GetConfig()
        await mountTab(info.id, cfg, initTitleRef.current, true, true)
      },
      closeTab: async (tabId: number, fromPTY = false) => {
        if (orderRef.current.length <= 1) return
        await removeTabUI(tabId, fromPTY)
      },
      nextTab: () => {
        const idx = orderRef.current.indexOf(activeRef.current)
        if (idx < 0) return
        activateTab(orderRef.current[(idx + 1) % orderRef.current.length])
      },
      prevTab: () => {
        const idx = orderRef.current.indexOf(activeRef.current)
        if (idx < 0) return
        const n = orderRef.current.length
        activateTab(orderRef.current[(idx + n - 1) % n])
      },
      moveTab: (delta: number) => {
        const idx = orderRef.current.indexOf(activeRef.current)
        if (idx < 0) return
        const next = idx + delta
        if (next < 0 || next >= orderRef.current.length) return
        const order = [...orderRef.current]
        const [item] = order.splice(idx, 1)
        order.splice(next, 0, item)
        orderRef.current = order
        syncTabList()
      },
    }

    async function init() {
      const [cfg, tabsResp, rawTitle] = await Promise.all([
        svc.GetConfig(),
        svc.GetTabs(),
        svc.GetInitialTitle(),
      ])
      if (disposed) return

      const initTitle = rawTitle || '~'
      initTitleRef.current = initTitle
      labelModeRef.current = tabsResp.label || 'path'

      keybindingsRef.current = cfg.keybindings ?? []
      baseFontSizeRef.current = cfg.fontSize

      setShowTitleBar(cfg.showTitleBar)
      setTitlePosition(cfg.titlePosition || 'center')
      document.documentElement.style.setProperty('--terminal-font-size', `${cfg.fontSize}px`)
      document.documentElement.style.setProperty('--terminal-padding-x', `${cfg.paddingX ?? 6}px`)
      document.documentElement.style.setProperty('--terminal-padding-y', `${cfg.paddingY ?? 6}px`)
      setTabsEnabled(tabsResp.enabled)
      setShowNewButton(tabsResp.showNewButton)
      setShowWhenSingle(tabsResp.showWhenSingle)
      setChromeReady(true)
      setTitle(initTitle)
      svc.SetTitle(initTitle).catch(() => {})

      if (!stack) return
      stack.innerHTML = ''
      orderRef.current = tabsResp.items.map((t) => t.id)
      activeRef.current = orderRef.current[0] ?? 0
      setActiveTab(activeRef.current)
      syncTabList()

      for (const item of tabsResp.items) {
        await mountTab(item.id, cfg, initTitle, item.id === activeRef.current, tabsResp.enabled)
        if (disposed) return
      }

      window.addEventListener('paste', onPaste, true)
      stack.addEventListener('contextmenu', onContextMenu)
    }

    init().catch(console.error)

    const onWindowResize = () => {
      for (const tab of tabsRef.current.values()) {
        tab.fitAddon?.fit()
      }
    }
    window.addEventListener('resize', onWindowResize)

    return () => {
      disposed = true
      offPtyData?.()
      offTabClosed?.()
      opsRef.current = null
      window.removeEventListener('resize', onWindowResize)
      window.removeEventListener('paste', onPaste, true)
      stack?.removeEventListener('contextmenu', onContextMenu)
      const tabs = tabsRef.current
      for (const tab of tabs.values()) {
        tab.term?.dispose()
      }
      tabs.clear()
    }
  }, [activateTab, syncTabList, onFontSize])

  const selectTab = (tabId: number) => activateTab(tabId)

  const reorderTabs = (fromIndex: number, toIndex: number) => {
    const entries = orderRef.current.map((id) => ({ id }))
    const next = reorderTabsAt(entries, fromIndex, toIndex)
    orderRef.current = next.map((e) => e.id)
    syncTabList()
  }

  return (
    <div className="app">
      {showTitleBar && (
        <TitleBar
          title={title}
          titlePosition={titlePosition}
          onToggleMaximize={() => svc.ToggleMaximize().catch(() => {})}
        />
      )}
      {tabsEnabled && chromeReady && (showWhenSingle || tabList.length > 1) && (
        <TabBar
          tabs={tabList}
          activeId={activeTab}
          showNewButton={showNewButton}
          onSelect={selectTab}
          onClose={(id) => opsRef.current?.closeTab(id).catch(() => {})}
          onNew={() => opsRef.current?.addTab().catch(() => {})}
          onReorder={reorderTabs}
        />
      )}
      <div className="terminal-container" ref={stackRef} />
    </div>
  )
}
