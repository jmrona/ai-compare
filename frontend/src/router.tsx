import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { AppShell } from '@/components/app/AppShell'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { HarnessDetailPage, HarnessListPage, HarnessNewPage } from '@/pages/HarnessesPage'
import { HistoryPage } from '@/pages/HistoryPage'
import { NewComparisonPage } from '@/pages/NewComparisonPage'
import { PricingPage } from '@/pages/PricingPage'
import { ReportPage } from '@/pages/ReportPage'
import { RunPage } from '@/pages/RunPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { SpikeTerminalPage } from '@/pages/SpikeTerminalPage'

const root = createRootRoute({ component: AppShell, notFoundComponent: NotFoundPage })

const routeTree = root.addChildren([
  createRoute({ getParentRoute: () => root, path: '/', component: NewComparisonPage }),
  createRoute({ getParentRoute: () => root, path: '/comparisons/$id', component: RunPage }),
  createRoute({ getParentRoute: () => root, path: '/history', component: HistoryPage }),
  createRoute({ getParentRoute: () => root, path: '/history/$id', component: ReportPage }),
  createRoute({ getParentRoute: () => root, path: '/harnesses', component: HarnessListPage }),
  createRoute({ getParentRoute: () => root, path: '/harnesses/new', component: HarnessNewPage }),
  createRoute({ getParentRoute: () => root, path: '/harnesses/$slug', component: HarnessDetailPage }),
  createRoute({ getParentRoute: () => root, path: '/pricing', component: PricingPage }),
  createRoute({ getParentRoute: () => root, path: '/settings', component: SettingsPage }),
  // Phase 0 spike pages, not in the navigation.
  createRoute({ getParentRoute: () => root, path: '/spike/terminal', component: SpikeTerminalPage }),
])

export const router = createRouter({ routeTree, defaultPreload: 'intent', scrollRestoration: true })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
