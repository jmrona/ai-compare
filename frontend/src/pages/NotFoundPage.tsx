import { Link } from '@tanstack/react-router'
import { TopBar } from '@/components/app/AppShell'

export function NotFoundPage() {
  return (
    <>
      <TopBar crumbs={[{ label: 'Not found' }]} />
      <div className="p-8 text-muted-foreground">
        This page does not exist. <Link to="/" className="text-foreground underline underline-offset-2">Start a new comparison</Link>.
      </div>
    </>
  )
}
