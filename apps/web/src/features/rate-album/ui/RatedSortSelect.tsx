import { CalendarDays, Clock, Star } from 'lucide-react'
import { PopoverSelect } from '@/shared/ui/PopoverSelect'
import { RATED_SORT_OPTIONS, type RatedSortKey } from '../model/ratedSort'

const ICONS: Record<RatedSortKey, typeof Clock> = {
  score: Star,
  rated: Clock,
  release: CalendarDays,
}

const OPTIONS = RATED_SORT_OPTIONS.map((o) => ({ ...o, icon: ICONS[o.key] }))

interface Props {
  value: RatedSortKey
  onChange: (s: RatedSortKey) => void
}

export function RatedSortSelect({ value, onChange }: Props) {
  return <PopoverSelect value={value} onChange={onChange} ariaLabel="정렬" options={OPTIONS} />
}
