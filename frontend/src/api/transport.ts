// The Connect transport and a client per service (proto/aicompare/v1). Queries go through
// connect-query (queries.ts); mutations and the event stream use these clients directly.

import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { CatalogService } from '@/gen/aicompare/v1/catalog_pb'
import { ComparisonService } from '@/gen/aicompare/v1/comparison_pb'
import { EventService } from '@/gen/aicompare/v1/events_pb'
import { PresetService } from '@/gen/aicompare/v1/preset_pb'
import { ProjectService } from '@/gen/aicompare/v1/project_pb'
import { ReportService } from '@/gen/aicompare/v1/report_pb'
import { SettingsService } from '@/gen/aicompare/v1/settings_pb'

export const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export const transport = createConnectTransport({
  baseUrl: new URL(API_BASE + '/rpc', window.location.href).toString(),
  // Read-only methods (no side effects) go as GET, so the browser can cache them.
  useHttpGet: true,
})

export const clients = {
  catalog: createClient(CatalogService, transport),
  projects: createClient(ProjectService, transport),
  presets: createClient(PresetService, transport),
  comparisons: createClient(ComparisonService, transport),
  events: createClient(EventService, transport),
  reports: createClient(ReportService, transport),
  settings: createClient(SettingsService, transport),
}
