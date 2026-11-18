import { useQuery } from '@tanstack/react-query'
import { fetchOnThisDay } from '@/entities/album/api/albumApi'

// date는 보는 사람 기준 오늘(YYYY-MM-DD). 자정을 넘기면 키가 바뀌어 새 날짜로 다시 불러온다.
export function useOnThisDay(date: string) {
  return useQuery({
    queryKey: ['albums', 'on-this-day', date],
    queryFn: () => fetchOnThisDay(date),
    staleTime: 60 * 60 * 1000,
  })
}
