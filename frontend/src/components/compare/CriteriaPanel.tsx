import { Plus, WandSparkles, X } from 'lucide-react'
import type { Criterion } from '@/api/types'
import { useGenerateCriteria } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Chip, ErrorNote, Panel } from '@/components/common/primitives'

export function CriteriaPanel({ prompt, judgeModel, value, onChange }: {
  prompt: string
  judgeModel: string
  value: Criterion[]
  onChange: (v: Criterion[]) => void
}) {
  const generate = useGenerateCriteria()
  const set = (i: number, patch: Partial<Criterion>) => onChange(value.map((c, k) => (k === i ? { ...c, ...patch } : c)))

  return (
    <Panel title="Acceptance criteria" right={<Chip>optional</Chip>}>
      <p className="max-w-[80ch] text-[12.5px] leading-relaxed text-muted-foreground">
        What the result must do to count as done. The report checks each side against this list; the agents never see it, so the task stays exactly your prompt.
        It cannot change once the comparison starts. Left empty, the judge model ({judgeModel}) writes the criteria from your prompt when the report is generated.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          disabled={!prompt.trim() || generate.isPending}
          onClick={() => generate.mutate(prompt, { onSuccess: onChange })}
          title={prompt.trim() ? 'Ask the judge model for criteria from the prompt' : 'Write the prompt first'}
        >
          <WandSparkles className="size-3.5" />{generate.isPending ? 'Generating…' : value.length ? 'Generate again' : 'Generate from the prompt'}
        </Button>
        <Button size="sm" variant="outline" onClick={() => onChange([...value, { text: '', required: true }])}>
          <Plus className="size-3.5" />Add criterion
        </Button>
        {value.length > 0 && <span className="text-xs text-dim">Required criteria are gates: a side that misses one cannot win.</span>}
      </div>
      {generate.error && <div className="mt-3"><ErrorNote error={generate.error} /></div>}
      {value.length > 0 && (
        <ol className="mt-3 grid gap-1.5">
          {value.map((c, i) => (
            <li key={i} className="grid grid-cols-[22px_minmax(0,1fr)] items-center gap-2 sm:grid-cols-[22px_minmax(0,1fr)_140px_32px]">
              <span className="tnum text-right font-mono text-xs text-dim">{i + 1}</span>
              <Input id={`criterion-${i}`} aria-label={`Criterion ${i + 1}`} value={c.text} placeholder="What the result must do" onChange={e => set(i, { text: e.target.value })} />
              <Select value={c.required ? 'required' : 'desirable'} onValueChange={v => set(i, { required: v === 'required' })}>
                <SelectTrigger aria-label={`Importance of criterion ${i + 1}`} className="col-start-2 sm:col-start-auto"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="required">Required</SelectItem>
                  <SelectItem value="desirable">Desirable</SelectItem>
                </SelectContent>
              </Select>
              <Button size="icon-sm" variant="ghost" className="col-start-2 sm:col-start-auto" aria-label={`Remove criterion ${i + 1}`} onClick={() => onChange(value.filter((_, k) => k !== i))}>
                <X className="size-3.5" />
              </Button>
            </li>
          ))}
        </ol>
      )}
    </Panel>
  )
}
