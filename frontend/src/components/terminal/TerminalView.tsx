import { useEffect, useRef } from 'react'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import type { TerminalSource } from '@/api/types'
import { cn } from '@/lib/utils'

const THEME = {
  background: '#0c0e10',
  foreground: '#d9dce2',
  cursor: '#d9dce2',
  cursorAccent: '#0c0e10',
  selectionBackground: '#6cb6ff55',
  black: '#16191d',
  brightBlack: '#6b7280',
  red: '#f07178',
  brightRed: '#f87171',
  green: '#63d38f',
  brightGreen: '#6fdc96',
  yellow: '#e8c35a',
  brightYellow: '#f1cf6a',
  blue: '#6cb6ff',
  brightBlue: '#8ec5ff',
  magenta: '#c4a1ff',
  brightMagenta: '#d4b2ff',
  cyan: '#7fd4e0',
  brightCyan: '#9be3ec',
  white: '#d9dce2',
  brightWhite: '#ffffff',
}

/**
 * A real terminal (xterm.js) wired to a TerminalSource.
 * On mount it receives the accumulated output, so reopening the tab rebuilds the screen.
 */
export function TerminalView({ source, readOnly, className }: { source: TerminalSource; readOnly?: boolean; className?: string }) {
  const host = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = host.current
    if (!el) return
    const term = new Terminal({
      theme: THEME,
      fontFamily: '"IBM Plex Mono", ui-monospace, Consolas, monospace',
      fontSize: 12.5,
      lineHeight: 1.3,
      cursorBlink: !readOnly,
      cursorStyle: 'block',
      disableStdin: readOnly,
      scrollback: 5000,
      allowProposedApi: false,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el)

    const refit = () => {
      try {
        fit.fit()
        source.resize(term.cols, term.rows)
      } catch {
        // The container may have no size yet (hidden tab).
      }
    }
    refit()
    // The font loads asynchronously; measure again once it is ready.
    document.fonts?.ready.then(refit)

    const unsubscribe = source.subscribe(chunk => term.write(chunk))
    const input = readOnly ? null : term.onData(data => source.send(data))
    const observer = new ResizeObserver(refit)
    observer.observe(el)
    if (!readOnly) term.focus()

    return () => {
      observer.disconnect()
      input?.dispose()
      unsubscribe()
      term.dispose()
    }
  }, [source, readOnly])

  // Padding lives on the wrapper: the fit addon sizes the terminal to its direct parent and ignores that parent's padding.
  return (
    <div className={cn('min-w-0 overflow-hidden bg-term py-2 pl-3', className ?? 'h-[380px]')}>
      <div ref={host} className="h-full w-full" />
    </div>
  )
}
