// Connect clients for the services in proto/aicompare/v1. The generated code in src/gen comes
// from `pnpm gen`; these helpers map its messages to the types the UI already uses.

import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { CatalogService } from '@/gen/aicompare/v1/catalog_pb'
import { ProjectService } from '@/gen/aicompare/v1/project_pb'
import type { Catalog as CatalogMessage, Price as PriceMessage } from '@/gen/aicompare/v1/catalog_pb'
import type { Catalog, Cli, FolderListing, Price, ProjectInspection, ProviderId } from './types'

const BASE = import.meta.env.VITE_API_BASE_URL || '/api'

const transport = createConnectTransport({
  baseUrl: new URL(BASE + '/rpc', window.location.href).toString(),
  // Read-only methods (no side effects) go as GET, so the browser can cache them.
  useHttpGet: true,
})

export const catalogService = createClient(CatalogService, transport)
export const projectService = createClient(ProjectService, transport)

export async function getCatalog(): Promise<Catalog> {
  const res = await catalogService.getCatalog({})
  return catalogFromProto(res.catalog)
}

export async function refreshCatalog(): Promise<Catalog> {
  const res = await catalogService.refreshCatalog({})
  return catalogFromProto(res.catalog)
}

export async function inspectProject(path: string): Promise<ProjectInspection> {
  const { inspection: i } = await projectService.inspectProject({ path })
  if (!i) throw new Error('The inspection response is empty')
  return {
    path: i.path,
    name: i.name,
    isGit: i.isGit,
    fileCount: Number(i.fileCount),
    sizeBytes: Number(i.sizeBytes),
    harnessFiles: i.harnessFiles.map(f => ({ path: f.path, readBy: f.readBy as Cli[] })),
    excluded: i.excluded,
    profile: {
      runtime: i.profile?.runtime ?? '',
      setup: i.profile?.setup ?? '',
      test: i.profile?.test ?? '',
      hiddenTestsPath: i.profile?.hiddenTestsPath ?? '',
    },
  }
}

export async function listFolders(path: string): Promise<FolderListing> {
  const res = await projectService.listFolders({ path })
  return { path: res.path, parent: res.parent, folders: res.folders.map(f => ({ name: f.name, path: f.path, isGit: f.isGit })) }
}

function catalogFromProto(c: CatalogMessage | undefined): Catalog {
  if (!c) throw new Error('The catalogue response is empty')
  return {
    source: 'models.dev',
    fetchedAt: c.fetchedAt ? timestampDate(c.fetchedAt).toISOString() : '',
    fromCache: c.fromCache,
    warning: c.warning || undefined,
    models: c.models.map(m => ({
      id: m.id,
      name: m.name,
      provider: m.provider as ProviderId,
      family: m.family || undefined,
      releaseDate: m.releaseDate || undefined,
      deprecated: m.deprecated,
      toolCall: m.toolCall,
      textOutput: m.textOutput,
      efforts: m.efforts,
      contextK: m.contextK,
      price: priceFromProto(m.price),
      longContext: m.longContext?.price ? { aboveTokens: m.longContext.aboveTokens, price: priceFromProto(m.longContext.price)! } : undefined,
    })),
  }
}

function priceFromProto(p: PriceMessage | undefined): Price | null {
  if (!p) return null
  return { input: p.input, cacheRead: p.cacheRead ?? null, cacheWrite: p.cacheWrite ?? null, output: p.output }
}
