import type { Terminal } from '@xterm/xterm'

export type TerminalAction =
  | 'Copy'
  | 'Paste'
  | 'Clear'
  | 'ScrollUp'
  | 'ScrollDown'
  | 'ScrollPageUp'
  | 'ScrollPageDown'
  | 'ScrollTop'
  | 'ScrollBottom'
  | 'FontIncrease'
  | 'FontDecrease'
  | 'FontReset'

export interface ConfigKeybinding {
  key: string
  mods: string[]
  action: string
}

interface Mods {
  ctrl: boolean
  shift: boolean
  alt: boolean
  meta: boolean
}

interface Binding {
  key: string
  mods: Mods
  action: TerminalAction
}

export interface TerminalActionContext {
  term: Terminal
  sendInput: (data: string) => void
  baseFontSize: number
  onFontSize: (size: number) => void
}

const isMac =
  navigator.platform.startsWith('Mac') ||
  navigator.userAgent.includes('Macintosh')

const TERMINAL_ACTIONS = new Set<string>([
  'Copy',
  'Paste',
  'Clear',
  'ScrollUp',
  'ScrollDown',
  'ScrollPageUp',
  'ScrollPageDown',
  'ScrollTop',
  'ScrollBottom',
  'FontIncrease',
  'FontDecrease',
  'FontReset',
])

function emptyMods(): Mods {
  return { ctrl: false, shift: false, alt: false, meta: false }
}

function modsFromConfig(mods: string[]): Mods {
  const out = emptyMods()
  for (const mod of mods) {
    switch (mod.toLowerCase()) {
      case 'ctrl':
      case 'control':
        out.ctrl = true
        break
      case 'shift':
        out.shift = true
        break
      case 'alt':
      case 'option':
        out.alt = true
        break
      case 'meta':
      case 'cmd':
      case 'super':
        out.meta = true
        break
    }
  }
  return out
}

function modsFromEvent(e: KeyboardEvent): Mods {
  return {
    ctrl: e.ctrlKey,
    shift: e.shiftKey,
    alt: e.altKey,
    meta: e.metaKey,
  }
}

function modsMatch(eventMods: Mods, bindingMods: Mods): boolean {
  return (
    eventMods.ctrl === bindingMods.ctrl &&
    eventMods.shift === bindingMods.shift &&
    eventMods.alt === bindingMods.alt &&
    eventMods.meta === bindingMods.meta
  )
}

function normalizeKey(key: string): string {
  switch (key) {
    case 'ArrowUp':
      return 'Up'
    case 'ArrowDown':
      return 'Down'
    case 'ArrowLeft':
      return 'Left'
    case 'ArrowRight':
      return 'Right'
    default:
      return key
  }
}

function keysMatch(bindingKey: string, eventKey: string): boolean {
  const a = normalizeKey(bindingKey)
  const b = normalizeKey(eventKey)
  if (a.length === 1 || b.length === 1) {
    return a.toLowerCase() === b.toLowerCase()
  }
  return a === b
}

function parseConfigBinding(kb: ConfigKeybinding): Binding | null {
  if (!TERMINAL_ACTIONS.has(kb.action)) {
    return null
  }
  return {
    key: kb.key,
    mods: modsFromConfig(kb.mods),
    action: kb.action as TerminalAction,
  }
}

function platformBindings(): Binding[] {
  if (isMac) {
    return [
      { key: 'k', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'Clear' },
      { key: 'v', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'Paste' },
      { key: 'c', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'Copy' },
      { key: '=', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'FontIncrease' },
      { key: '+', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'FontIncrease' },
      { key: '-', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'FontDecrease' },
      { key: '0', mods: { ctrl: false, shift: false, alt: false, meta: true }, action: 'FontReset' },
    ]
  }

  return [
    { key: 'Insert', mods: { ctrl: true, shift: false, alt: false, meta: false }, action: 'Copy' },
    { key: 'Insert', mods: { ctrl: false, shift: true, alt: false, meta: false }, action: 'Paste' },
  ]
}

export function buildBindingList(config: ConfigKeybinding[]): Binding[] {
  const seen = new Set<string>()
  const out: Binding[] = []

  const add = (binding: Binding) => {
    const id = `${binding.action}:${binding.key}:${binding.mods.ctrl}:${binding.mods.shift}:${binding.mods.alt}:${binding.mods.meta}`
    if (seen.has(id)) return
    seen.add(id)
    out.push(binding)
  }

  for (const binding of platformBindings()) {
    add(binding)
  }
  for (const kb of config) {
    const binding = parseConfigBinding(kb)
    if (binding) add(binding)
  }
  return out
}

export function matchBinding(
  e: KeyboardEvent,
  bindings: Binding[],
): TerminalAction | null {
  const eventMods = modsFromEvent(e)
  const eventKey = normalizeKey(e.key)

  for (const binding of bindings) {
    if (!keysMatch(binding.key, eventKey)) continue
    if (!modsMatch(eventMods, binding.mods)) continue
    return binding.action
  }
  return null
}

async function pasteFromClipboard(ctx: TerminalActionContext): Promise<void> {
  try {
    const text = await navigator.clipboard.readText()
    if (text) ctx.sendInput(text)
  } catch {
    /* clipboard blocked */
  }
}

export function handleTerminalAction(
  action: TerminalAction,
  ctx: TerminalActionContext,
): boolean {
  const { term } = ctx

  switch (action) {
    case 'Copy':
      if (!term.hasSelection()) {
        return true
      }
      navigator.clipboard.writeText(term.getSelection()).catch(() => {})
      return false

    case 'Paste':
      pasteFromClipboard(ctx).catch(() => {})
      return false

    case 'Clear':
      term.clear()
      term.scrollToBottom()
      return false

    case 'ScrollUp':
      term.scrollLines(-1)
      return false

    case 'ScrollDown':
      term.scrollLines(1)
      return false

    case 'ScrollPageUp':
      term.scrollPages(-1)
      return false

    case 'ScrollPageDown':
      term.scrollPages(1)
      return false

    case 'ScrollTop':
      term.scrollToTop()
      return false

    case 'ScrollBottom':
      term.scrollToBottom()
      return false

    case 'FontIncrease': {
      const next = Math.min(32, (term.options.fontSize ?? ctx.baseFontSize) + 1)
      term.options.fontSize = next
      ctx.onFontSize(next)
      return false
    }

    case 'FontDecrease': {
      const next = Math.max(8, (term.options.fontSize ?? ctx.baseFontSize) - 1)
      term.options.fontSize = next
      ctx.onFontSize(next)
      return false
    }

    case 'FontReset':
      term.options.fontSize = ctx.baseFontSize
      ctx.onFontSize(ctx.baseFontSize)
      return false

    default:
      return true
  }
}
