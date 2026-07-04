// Wails v3 bindings for TerminalService (github.com/termizard/termizard/internal/ui/wails).
// Calls are routed to the correct PTY session server-side via window context.
import { Call, type CancellablePromise } from '@wailsio/runtime'

const pkg = 'github.com/termizard/termizard/internal/ui/wails.TerminalService'

export interface XTermTheme {
  background: string
  foreground: string
  cursor: string
  selection: string
  black: string
  red: string
  green: string
  yellow: string
  blue: string
  magenta: string
  cyan: string
  white: string
  brightBlack: string
  brightRed: string
  brightGreen: string
  brightYellow: string
  brightBlue: string
  brightMagenta: string
  brightCyan: string
  brightWhite: string
}

export interface XTermConfig {
  fontSize: number
  fontFamily: string
  cursorStyle: 'block' | 'bar' | 'underline'
  cursorBlink: boolean
  showTitleBar: boolean
  theme: XTermTheme
}

export function GetConfig(): CancellablePromise<XTermConfig> {
  return Call.ByName(`${pkg}.GetConfig`)
}

export function GetInitialTitle(): CancellablePromise<string> {
  return Call.ByName(`${pkg}.GetInitialTitle`)
}

export function SetTitle(title: string): CancellablePromise<void> {
  return Call.ByName(`${pkg}.SetTitle`, title)
}

export function SendInput(data: string): CancellablePromise<void> {
  return Call.ByName(`${pkg}.SendInput`, data)
}

export function Resize(cols: number, rows: number): CancellablePromise<void> {
  return Call.ByName(`${pkg}.Resize`, cols, rows)
}

export function Ready(cols: number, rows: number): CancellablePromise<void> {
  return Call.ByName(`${pkg}.Ready`, cols, rows)
}

export function NewWindow(): CancellablePromise<void> {
  return Call.ByName(`${pkg}.NewWindow`)
}
