// Wails v3 bindings for TerminalService (github.com/termizard/termizard/internal/ui/wails).
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

export type TitlePosition = 'left' | 'center' | 'right'
export type TabLabelMode = 'path' | 'index'

export interface XTermConfig {
  fontSize: number
  fontFamily: string
  cursorStyle: 'block' | 'bar' | 'underline'
  cursorBlink: boolean
  showTitleBar: boolean
  titlePosition: TitlePosition
  paddingX: number
  paddingY: number
  theme: XTermTheme
  keybindings: KeyBinding[]
}

export interface KeyBinding {
  key: string
  mods: string[]
  action: string
}

export interface TabInfo {
  id: number
  title?: string
}

export interface TabsResponse {
  enabled: boolean
  label: TabLabelMode
  showNewButton: boolean
  showWhenSingle: boolean
  items: TabInfo[]
}

export function GetConfig(): CancellablePromise<XTermConfig> {
  return Call.ByName(`${pkg}.GetConfig`)
}

export function GetTabs(): CancellablePromise<TabsResponse> {
  return Call.ByName(`${pkg}.GetTabs`)
}

export function GetInitialTitle(): CancellablePromise<string> {
  return Call.ByName(`${pkg}.GetInitialTitle`)
}

export function SetTitle(title: string): CancellablePromise<void> {
  return Call.ByName(`${pkg}.SetTitle`, title)
}

export function ToggleMaximize(): CancellablePromise<void> {
  return Call.ByName(`${pkg}.ToggleMaximize`)
}

export function SendInput(tabId: number, data: string): CancellablePromise<void> {
  return Call.ByName(`${pkg}.SendInput`, tabId, data)
}

export function Resize(tabId: number, cols: number, rows: number): CancellablePromise<void> {
  return Call.ByName(`${pkg}.Resize`, tabId, cols, rows)
}

export function Ready(tabId: number, cols: number, rows: number): CancellablePromise<void> {
  return Call.ByName(`${pkg}.Ready`, tabId, cols, rows)
}

export function NewTab(): CancellablePromise<TabInfo> {
  return Call.ByName(`${pkg}.NewTab`)
}

export function CloseTab(tabId: number): CancellablePromise<void> {
  return Call.ByName(`${pkg}.CloseTab`, tabId)
}

export function NewWindow(): CancellablePromise<void> {
  return Call.ByName(`${pkg}.NewWindow`)
}
