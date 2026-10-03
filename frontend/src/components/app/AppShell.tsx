import type { ReactNode } from 'react'
import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { CircleDollarSign, Columns2, History, Layers, SlidersHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useActiveComparison, useCatalog } from '@/api/queries'
import { useEventStream } from '@/api/events'
import { formatTime } from '@/lib/format'
import { Dot } from '@/components/common/primitives'

const NAV = [
  { to: '/', label: 'Compare', icon: Columns2, match: (p: string) => p === '/' || p.startsWith('/comparisons') },
  { to: '/history', label: 'History', icon: History, match: (p: string) => p.startsWith('/history') },
  { to: '/harnesses', label: 'Harnesses', icon: Layers, match: (p: string) => p.startsWith('/harnesses') },
  { to: '/pricing', label: 'Pricing', icon: CircleDollarSign, match: (p: string) => p.startsWith('/pricing') },
  { to: '/settings', label: 'Settings', icon: SlidersHorizontal, match: (p: string) => p.startsWith('/settings') },
] as const

export function AppShell() {
  const pathname = useRouterState({ select: s => s.location.pathname })
  return (
    <div className="grid min-h-dvh grid-rows-[auto_1fr] sm:grid-cols-[64px_minmax(0,1fr)] sm:grid-rows-1">
      <nav aria-label="Main navigation" className="flex items-center gap-1 overflow-x-auto border-b bg-background px-2 py-1.5 sm:sticky sm:top-0 sm:h-dvh sm:flex-col sm:border-r sm:border-b-0 sm:px-0 sm:py-3">
        <Link to="/" aria-label="ai-compare home" className="mr-2 flex size-6 shrink-0 overflow-hidden sm:mr-0 sm:mb-4">
          <span className="w-1/2 bg-side-a" />
          <span className="w-1/2 bg-side-b" />
        </Link>
        {NAV.map(({ to, label, icon: Icon, match }) => (
          <Link
            key={to}
            to={to}
            className={cn(
              'flex shrink-0 flex-col items-center gap-0.5 px-2 py-1.5 text-[10px] transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/60 sm:w-[52px]',
              match(pathname) ? 'bg-raise text-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Icon className="size-[17px]" strokeWidth={1.75} />
            {label}
          </Link>
        ))}
      </nav>
      {/* On desktop the app is exactly one screen high: pages scroll inside main, and pages that
          fill the screen (the live run) give each pane its own scroll. */}
      <div className="flex min-h-dvh min-w-0 flex-col sm:h-dvh sm:min-h-0">
        <main className="flex min-h-0 min-w-0 flex-1 flex-col sm:overflow-y-auto">
          <Outlet />
        </main>
        <StatusBar />
      </div>
      <Toaster
        theme="dark"
        position="bottom-right"
        offset={{ bottom: 36, right: 16 }}
        toastOptions={{
          classNames: {
            toast: '!rounded-none !border !border-border !bg-panel !text-foreground !font-sans',
            description: '!text-muted-foreground !font-mono !text-[11.5px]',
            actionButton: '!rounded-none !bg-foreground !text-background',
          },
        }}
      />
    </div>
  )
}

type CrumbTarget = '/' | '/history' | '/harnesses'

export function TopBar({ crumbs, children }: { crumbs: { label: string; to?: CrumbTarget }[]; children?: ReactNode }) {
  return (
    <div className="sticky top-0 z-30 flex min-h-[46px] flex-wrap items-center gap-3 border-b bg-background/95 px-4 py-2 backdrop-blur">
      <nav aria-label="Breadcrumb" className="min-w-0 truncate text-[13px] text-muted-foreground">
        {crumbs.map((c, i) => (
          <span key={i}>
            {i > 0 && <span className="mx-1.5 text-dim">/</span>}
            {i === crumbs.length - 1 ? (
              <span className="font-medium text-foreground">{c.label}</span>
            ) : c.to ? (
              <Link to={c.to} className="hover:text-foreground">{c.label}</Link>
            ) : (
              c.label
            )}
          </span>
        ))}
      </nav>
      <div className="ml-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  )
}

function StatusBar() {
  // The app's single live connection: every change to a comparison arrives through it.
  const stream = useEventStream()
  const { data: active } = useActiveComparison()
  const { data: catalog } = useCatalog()
  return (
    <footer className="sticky bottom-0 z-30 flex flex-wrap gap-x-5 gap-y-1 border-t bg-background px-4 py-1 pb-[calc(4px+env(safe-area-inset-bottom))] font-mono text-[11.5px] text-muted-foreground">
      {stream === 'live' ? (
        <span className="text-ok" title="Changes arrive as they happen (EventService.Watch)">● live</span>
      ) : (
        <span className="text-warn">● {stream === 'connecting' ? 'connecting…' : 'reconnecting…'}</span>
      )}
      <span>proxy :4701</span>
      {active ? (
        <Link to="/comparisons/$id" params={{ id: active.id }} className="inline-flex items-center gap-1.5 text-warn hover:underline">
          <Dot tone="warn" live />
          comparison #{active.id} running
        </Link>
      ) : (
        <span>no comparisons running</span>
      )}
      {catalog && (
        <span className="ml-auto">
          models.dev · {catalog.fromCache ? 'cached at' : 'refreshed at'} {formatTime(catalog.fetchedAt)}
        </span>
      )}
    </footer>
  )
}
