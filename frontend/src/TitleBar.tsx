interface Props {
  title: string
  titlePosition: 'left' | 'center' | 'right'
  onToggleMaximize: () => void
}

const isMac = navigator.platform.startsWith('Mac') ||
              navigator.userAgent.includes('Macintosh')

const isWin = navigator.userAgent.includes('Windows')

export function TitleBar({ title, titlePosition, onToggleMaximize }: Props) {
  const classes = [
    'titlebar',
    isMac ? 'titlebar--mac' : isWin ? 'titlebar--win' : 'titlebar--linux',
    `titlebar--align-${titlePosition}`,
  ].join(' ')

  if (isMac) {
    return (
      <div
        className={classes}
        onDoubleClick={onToggleMaximize}
        title="Double-click to maximize"
      >
        <div className="titlebar-mac-gutter" aria-hidden="true" />
        <div className="titlebar-mac-body">
          <span className="titlebar-title">{title}</span>
        </div>
      </div>
    )
  }

  return (
    <div
      className={classes}
      onDoubleClick={onToggleMaximize}
      title="Double-click to maximize"
    >
      <div className="titlebar-side titlebar-side--left" aria-hidden="true" />
      <span className="titlebar-title">{title}</span>
      <div className="titlebar-side titlebar-side--right" aria-hidden="true" />
    </div>
  )
}
