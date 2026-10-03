import type { ReactNode } from 'react'
import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface TreeFile {
  path: string
  aside?: ReactNode
}

interface TreeFolder {
  name: string
  path: string
  folders: TreeFolder[]
  files: { name: string; file: TreeFile }[]
}

function buildTree(files: TreeFile[]): TreeFolder {
  const root: TreeFolder = { name: '', path: '', folders: [], files: [] }
  for (const file of files) {
    const parts = file.path.split('/')
    let node = root
    for (const part of parts.slice(0, -1)) {
      let next = node.folders.find(f => f.name === part)
      if (!next) {
        next = { name: part, path: node.path ? `${node.path}/${part}` : part, folders: [], files: [] }
        node.folders.push(next)
      }
      node = next
    }
    node.files.push({ name: parts.at(-1) ?? file.path, file })
  }
  const sort = (node: TreeFolder) => {
    node.folders.sort((a, b) => a.name.localeCompare(b.name))
    node.files.sort((a, b) => a.name.localeCompare(b.name))
    node.folders.forEach(sort)
  }
  sort(root)
  return root
}

const ancestorsOf = (path: string) => {
  const parts = path.split('/').slice(0, -1)
  return parts.map((_, i) => parts.slice(0, i + 1).join('/'))
}

const indent = (depth: number) => ({ paddingLeft: `${8 + depth * 14}px` })

export function FileTree({ files, selected, onSelect, className }: {
  files: TreeFile[]
  selected?: string | null
  onSelect: (path: string) => void
  className?: string
}) {
  const tree = useMemo(() => buildTree(files), [files])
  const opened = useMemo(() => new Set(selected ? ancestorsOf(selected) : []), [selected])
  const [toggled, setToggled] = useState<Record<string, boolean>>({})
  const isOpen = (path: string) => toggled[path] ?? opened.has(path)

  const renderFolder = (node: TreeFolder, depth: number): ReactNode => (
    <>
      {node.folders.map(folder => {
        const open = isOpen(folder.path)
        return (
          <li key={`d:${folder.path}`}>
            <button
              type="button"
              aria-expanded={open}
              onClick={() => setToggled(t => ({ ...t, [folder.path]: !open }))}
              className="flex w-full items-center gap-1 py-[3px] pr-2 text-left text-muted-foreground outline-none hover:bg-raise hover:text-foreground focus-visible:bg-raise"
              style={indent(depth)}
              title={folder.path}
            >
              {open ? <ChevronDown className="size-3.5 shrink-0 text-dim" /> : <ChevronRight className="size-3.5 shrink-0 text-dim" />}
              {open ? <FolderOpen className="size-3.5 shrink-0" /> : <Folder className="size-3.5 shrink-0" />}
              <span className="min-w-0 truncate">{folder.name}</span>
            </button>
            {open && <ul>{renderFolder(folder, depth + 1)}</ul>}
          </li>
        )
      })}
      {node.files.map(({ name, file }) => (
        <li key={`f:${file.path}`}>
          <button
            type="button"
            aria-current={selected === file.path ? 'true' : undefined}
            onClick={() => onSelect(file.path)}
            className={cn(
              'flex w-full items-center gap-1 py-[3px] pr-2 text-left outline-none focus-visible:bg-raise',
              selected === file.path ? 'bg-raise text-foreground' : 'text-muted-foreground hover:bg-raise/60 hover:text-foreground',
            )}
            style={indent(depth)}
            title={file.path}
          >
            <span className="size-3.5 shrink-0" />
            <FileText className="size-3.5 shrink-0 text-dim" />
            <span className="min-w-0 truncate">{name}</span>
            {file.aside != null && <span className="ml-auto shrink-0 pl-2 text-[10px] text-dim">{file.aside}</span>}
          </button>
        </li>
      ))}
    </>
  )

  return <ul className={cn('font-mono text-xs', className)}>{renderFolder(tree, 0)}</ul>
}
