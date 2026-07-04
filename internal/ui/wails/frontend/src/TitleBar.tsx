interface Props {
  title: string
  onToggleMaximize: () => void
}

const isMac = navigator.platform.startsWith('Mac') ||
              navigator.userAgent.includes('Macintosh')

export function TitleBar({ title, onToggleMaximize }: Props) {
  return (
    <div
      className={`titlebar${isMac ? ' titlebar--mac' : ''}`}
      onDoubleClick={onToggleMaximize}
      title="Double-click to maximize"
    >
      <span className="titlebar-title">{title}</span>
    </div>
  )
}
