// Phase 0 spike, point 4: a real container TTY in the browser. Not linked from the navigation;
// open /spike/terminal directly. It always talks to the real backend, even with mocks on.

import { useMemo, useState } from 'react'
import { Plug, Unplug } from 'lucide-react'
import { websocketTerminal } from '@/api/http'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TopBar } from '@/components/app/AppShell'
import { Chip, Field } from '@/components/common/primitives'
import { TerminalView } from '@/components/terminal/TerminalView'

const CHECKS = [
  'Typing: run ls, echo hello, or open vi.',
  'Resize: change the window size, then run tput cols; lines; it should match.',
  'Ctrl+C: run sleep 100, press Ctrl+C, and the prompt should come back.',
  'Interactive apps and colours: run node, try 1+1, then .exit; run ls --color /.',
]

export function SpikeTerminalPage() {
  const [image, setImage] = useState('node:22-bookworm-slim')
  // Each connection is a new session (and a new container); the image is fixed when connecting.
  const [session, setSession] = useState<{ id: number; image: string } | null>(null)
  const source = useMemo(
    () => (session ? websocketTerminal(`/spike/terminal?image=${encodeURIComponent(session.image)}`) : null),
    [session],
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TopBar crumbs={[{ label: 'Spike' }, { label: 'Container terminal' }]}>
        <Chip>phase 0 · point 4</Chip>
      </TopBar>
      <div className="flex flex-wrap items-end gap-3 border-b px-4 py-3">
        <Field label="Image" htmlFor="spike-image" className="w-72">
          <Input id="spike-image" className="font-mono" value={image} onChange={e => setImage(e.target.value)} disabled={!!session} />
        </Field>
        {session ? (
          <Button variant="outline" onClick={() => setSession(null)}><Unplug className="size-3.5" />Disconnect</Button>
        ) : (
          <Button onClick={() => setSession({ id: Date.now(), image })}><Plug className="size-3.5" />Start bash in a container</Button>
        )}
        <ul className="ml-auto grid gap-0.5 text-xs text-muted-foreground">
          {CHECKS.map(c => <li key={c}>· {c}</li>)}
        </ul>
      </div>
      {source ? (
        <TerminalView key={session?.id} source={source} className="min-h-[380px] flex-1" />
      ) : (
        <div className="grid flex-1 place-items-center bg-term p-8 text-center text-sm text-muted-foreground">
          Starts a throwaway container with bash and a TTY. It is removed when you disconnect or close the tab.
        </div>
      )}
    </div>
  )
}
