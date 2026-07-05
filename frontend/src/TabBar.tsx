import { useCallback, useEffect, useRef, useState } from 'react'

interface Tab {
  id: number
  title: string
}

interface Props {
  tabs: Tab[]
  activeId: number
  showNewButton: boolean
  onSelect: (id: number) => void
  onClose: (id: number) => void
  onNew: () => void
  onReorder: (fromIndex: number, toIndex: number) => void
}

const DRAG_THRESHOLD = 6

interface SlotRect {
  left: number
  right: number
  mid: number
  width: number
}

function slotIndexAtX(slots: SlotRect[], clientX: number): number {
  for (let i = 0; i < slots.length; i++) {
    if (clientX < slots[i].mid) return i
  }
  return slots.length - 1
}

function shiftForIndex(index: number, from: number, over: number, tabWidth: number): number {
  if (index === from) return 0
  if (from < over && index > from && index <= over) return -tabWidth
  if (from > over && index >= over && index < from) return tabWidth
  return 0
}

export function TabBar({
  tabs,
  activeId,
  showNewButton,
  onSelect,
  onClose,
  onNew,
  onReorder,
}: Props) {
  const tabbarRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const tabsRowRef = useRef<HTMLDivElement>(null)
  const wrapRefs = useRef<Map<number, HTMLDivElement>>(new Map())

  const [isDragging, setIsDragging] = useState(false)
  const [overflow, setOverflow] = useState(false)

  const updateActivePin = useCallback(() => {
    const scroll = scrollRef.current
    const el = wrapRefs.current.get(activeId)
    if (!scroll || !el) return

    el.classList.remove('tabbar-item-wrap--pin-left', 'tabbar-item-wrap--pin-right')
    if (!overflow) return

    const sl = scroll.scrollLeft
    const cw = scroll.clientWidth
    const tabLeft = el.offsetLeft
    const tabRight = tabLeft + el.offsetWidth

    const cutLeft = tabLeft < sl - 1
    const cutRight = tabRight > sl + cw + 1

    if (cutLeft && !cutRight) {
      el.classList.add('tabbar-item-wrap--pin-left')
    } else if (cutRight && !cutLeft) {
      el.classList.add('tabbar-item-wrap--pin-right')
    } else if (cutLeft && cutRight) {
      el.classList.add('tabbar-item-wrap--pin-left')
    }
  }, [activeId, overflow])

  const scrollActiveIntoView = useCallback(() => {
    const scroll = scrollRef.current
    const el = wrapRefs.current.get(activeId)
    if (!scroll || !el) return

    if (!overflow) {
      el.scrollIntoView({ inline: 'nearest', block: 'nearest', behavior: 'smooth' })
      return
    }

    const tabLeft = el.offsetLeft
    const tabRight = tabLeft + el.offsetWidth
    const sl = scroll.scrollLeft
    const cw = scroll.clientWidth

    if (tabLeft < sl) {
      scroll.scrollLeft = tabLeft
    } else if (tabRight > sl + cw) {
      scroll.scrollLeft = tabRight - cw
    }
    updateActivePin()
  }, [activeId, overflow, updateActivePin])

  useEffect(() => {
    scrollActiveIntoView()
  }, [activeId, tabs.length, scrollActiveIntoView])

  useEffect(() => {
    const row = tabsRowRef.current
    const scroll = scrollRef.current
    if (!row || !scroll) return

    const checkOverflow = () => {
      setOverflow(row.scrollWidth > scroll.clientWidth + 2)
    }
    checkOverflow()
    const ro = new ResizeObserver(checkOverflow)
    ro.observe(row)
    ro.observe(scroll)
    return () => ro.disconnect()
  }, [tabs])

  useEffect(() => {
    const scroll = scrollRef.current
    if (!scroll) return
    const onScroll = () => updateActivePin()
    scroll.addEventListener('scroll', onScroll, { passive: true })
    updateActivePin()
    return () => scroll.removeEventListener('scroll', onScroll)
  }, [activeId, overflow, tabs.length, updateActivePin])

  const clearTransforms = useCallback(() => {
    wrapRefs.current.forEach((el) => {
      el.style.transform = ''
      el.style.transition = ''
      el.classList.remove('tabbar-item-wrap--dragging')
    })
    tabbarRef.current?.classList.remove('tabbar--dragging')
    setIsDragging(false)
  }, [])

  const applyPreview = useCallback((
    from: number,
    over: number,
    dragDx: number,
    slots: SlotRect[],
    dragTabId: number,
  ) => {
    const dragWidth = slots[from]?.width ?? 0
    wrapRefs.current.forEach((el, id) => {
      const index = tabs.findIndex((t) => t.id === id)
      if (index < 0) return
      if (id === dragTabId) {
        el.style.transition = 'none'
        el.style.transform = `translateX(${dragDx}px)`
        return
      }
      const dx = shiftForIndex(index, from, over, dragWidth)
      el.style.transition = 'transform 0.18s cubic-bezier(0.2, 0, 0, 1)'
      el.style.transform = dx === 0 ? '' : `translateX(${dx}px)`
    })
  }, [tabs])

  const bindWrapRef = useCallback((id: number, el: HTMLDivElement | null) => {
    if (el) wrapRefs.current.set(id, el)
    else wrapRefs.current.delete(id)
  }, [])

  const onTabPointerDown = useCallback((e: React.PointerEvent, tabId: number) => {
    if (e.button !== 0) return
    if ((e.target as HTMLElement).closest('.tabbar-item-close')) return

    const from = tabs.findIndex((t) => t.id === tabId)
    if (from < 0 || !scrollRef.current || !tabsRowRef.current) return

    const startX = e.clientX
    const startY = e.clientY
    let dragging = false
    let over = from
    let slots: SlotRect[] = []

    const measureSlots = () => {
      slots = tabs.map((tab) => {
        const el = wrapRefs.current.get(tab.id)
        const rect = el?.getBoundingClientRect()
        if (!rect) return { left: 0, right: 0, mid: 0, width: 0 }
        return {
          left: rect.left,
          right: rect.right,
          mid: rect.left + rect.width / 2,
          width: rect.width,
        }
      })
    }

    const onMove = (ev: PointerEvent) => {
      const dx = ev.clientX - startX
      const dy = ev.clientY - startY

      if (!dragging) {
        if (Math.hypot(dx, dy) < DRAG_THRESHOLD) return
        dragging = true
        measureSlots()
        tabbarRef.current?.classList.add('tabbar--dragging')
        wrapRefs.current.get(tabId)?.classList.add('tabbar-item-wrap--dragging')
        setIsDragging(true)
      }

      ev.preventDefault()

      const nextOver = slotIndexAtX(slots, ev.clientX)
      if (nextOver !== over) over = nextOver
      applyPreview(from, over, dx, slots, tabId)

      // Edge auto-scroll while dragging.
      const sc = scrollRef.current!
      const scRect = sc.getBoundingClientRect()
      if (ev.clientX < scRect.left + 40) sc.scrollLeft -= 8
      else if (ev.clientX > scRect.right - 40) sc.scrollLeft += 8
    }

    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)

      if (dragging) {
        clearTransforms()
        if (from !== over) onReorder(from, over)
        if (tabId !== activeId) onSelect(tabId)
      } else {
        // Click without drag — macOS-style: activate tab on release.
        if (tabId !== activeId) onSelect(tabId)
      }
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }, [activeId, applyPreview, clearTransforms, onReorder, onSelect, tabs])

  return (
    <div
      ref={tabbarRef}
      className={`tabbar${isDragging ? ' tabbar--dragging' : ''}`}
      role="tablist"
    >
      <div className="tabbar-scroll" ref={scrollRef}>
        <div className={`tabbar-tabs${overflow ? ' tabbar-tabs--overflow' : ''}`} ref={tabsRowRef}>
          {tabs.map((tab) => {
            const isActive = tab.id === activeId
            return (
              <div
                key={tab.id}
                ref={(el) => bindWrapRef(tab.id, el)}
                data-tab-id={tab.id}
                role="presentation"
                className={`tabbar-item-wrap${isActive ? ' tabbar-item-wrap--active' : ''}`}
                onPointerDown={(e) => onTabPointerDown(e, tab.id)}
              >
                <div
                  role="tab"
                  className="tabbar-item"
                  aria-selected={isActive}
                >
                  {tabs.length > 1 && (
                    <button
                      type="button"
                      className="tabbar-item-close"
                      aria-label="Close tab"
                      onPointerDown={(ev) => ev.stopPropagation()}
                      onClick={(ev) => {
                        ev.stopPropagation()
                        onClose(tab.id)
                      }}
                    >
                      ×
                    </button>
                  )}
                  <span className="tabbar-item-label">{tab.title}</span>
                </div>
              </div>
            )
          })}
        </div>
      </div>
      {showNewButton && (
        <button
          type="button"
          className="tabbar-new"
          aria-label="New tab"
          title="New tab"
          onClick={onNew}
        >
          +
        </button>
      )}
    </div>
  )
}
