interface Props {
  title: string
}

const isMac = navigator.platform.startsWith('Mac') ||
              navigator.userAgent.includes('Macintosh')

export function TitleBar({ title }: Props) {
  return (
    <div className={`titlebar${isMac ? ' titlebar--mac' : ''}`}>
      <span className="titlebar-title">{title}</span>
    </div>
  )
}
