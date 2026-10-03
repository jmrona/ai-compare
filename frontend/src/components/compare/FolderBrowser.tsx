// Folder browser for the project path. Browsers never reveal a folder's absolute path, so the
// folders are listed by the backend (ProjectService.ListFolders), which reads the host's disk
// read-only through a helper container.

import { useState } from 'react'
import { ArrowUp, Folder, FolderGit2, Home } from 'lucide-react'
import { useFolders } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Chip, ErrorNote, LoadingRows } from '@/components/common/primitives'

export function FolderBrowser({ open, onOpenChange, startPath, onSelect }: {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Where to start; empty starts at the user's home folder. */
  startPath: string
  onSelect: (path: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        {/* Mounted only while open, so each opening starts from startPath. */}
        {open && <Browser startPath={startPath} onSelect={p => { onSelect(p); onOpenChange(false) }} />}
      </DialogContent>
    </Dialog>
  )
}

function Browser({ startPath, onSelect }: { startPath: string; onSelect: (path: string) => void }) {
  const [path, setPath] = useState(startPath)
  const { data, error, isFetching } = useFolders(path, true)

  return (
    <>
      <DialogHeader>
        <DialogTitle>Choose the project folder</DialogTitle>
        <DialogDescription>Folders on this machine that Docker can see. The folder is only read, never changed.</DialogDescription>
      </DialogHeader>

      <div className="flex items-center gap-1.5">
        <Button size="icon-sm" variant="outline" aria-label="Parent folder" disabled={!data?.parent} onClick={() => data && setPath(data.parent)}>
          <ArrowUp className="size-3.5" />
        </Button>
        <Button size="icon-sm" variant="outline" aria-label="Home folder" onClick={() => setPath('')}>
          <Home className="size-3.5" />
        </Button>
        <span className="min-w-0 flex-1 truncate border bg-term px-2 py-1 font-mono text-xs" title={data?.path ?? path}>
          {data?.path ?? (path || '~')}
        </span>
      </div>

      <div className="h-80 overflow-y-auto border bg-term" aria-busy={isFetching}>
        {error && (
          <div className="grid gap-2 p-3">
            <ErrorNote error={error} />
            <Button size="sm" variant="outline" className="w-fit" onClick={() => setPath('')}>Go to the home folder</Button>
          </div>
        )}
        {!data && !error && <div className="p-3"><LoadingRows rows={6} /></div>}
        {data && !error && data.folders.length === 0 && <p className="p-3 text-xs text-muted-foreground">No sub-folders.</p>}
        {data && !error && (
          <ul className={isFetching ? 'opacity-60' : undefined}>
            {data.folders.map(f => (
              <li key={f.path}>
                <button
                  type="button"
                  onClick={() => setPath(f.path)}
                  onDoubleClick={() => onSelect(f.path)}
                  className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-1.5 text-left font-mono text-[12.5px] outline-none hover:bg-raise focus-visible:bg-raise"
                >
                  {f.isGit ? <FolderGit2 className="size-3.5 shrink-0 text-ok" /> : <Folder className="size-3.5 shrink-0 text-dim" />}
                  <span className="truncate">{f.name}</span>
                  {f.isGit && <Chip tone="ok" className="ml-auto">git</Chip>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <DialogFooter className="items-center sm:justify-between">
        <span className="text-xs text-muted-foreground">Click to open a folder · double-click to choose it</span>
        <Button disabled={!data || !!error} onClick={() => data && onSelect(data.path)}>Select this folder</Button>
      </DialogFooter>
    </>
  )
}
