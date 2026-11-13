import { useMemo } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchRatings, fetchRatedAlbums, setRating, removeRating } from '@/features/rate-album/api/ratingApi'
import { toRatedSortParam, type RatedSortKey } from './ratedSort'
import { useToast } from '@/shared/ui/toast'

export function useRatings(enabled: boolean) {
  return useQuery({ queryKey: ['ratings'], queryFn: fetchRatings, enabled, staleTime: 60_000 })
}

// Full rated-album rows (for the my-page grid).
export function useRatedAlbums(enabled: boolean, sort: RatedSortKey) {
  return useQuery({
    queryKey: ['rated-albums', sort],
    queryFn: () => fetchRatedAlbums(toRatedSortParam(sort)),
    enabled,
    staleTime: 60_000,
    placeholderData: keepPreviousData, // 정렬을 바꿀 때 목록이 스켈레톤으로 깜빡이지 않게
  })
}

// album id → score in half-stars (empty when logged out).
export function useRatingMap(enabled: boolean): Map<string, number> {
  const { data } = useRatings(enabled)
  return useMemo(() => new Map((data ?? []).map((r) => [r.album_id, r.score])), [data])
}

// score 0 clears the rating (clicking the same half-star again).
export function useSetRating() {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ id, score }: { id: string; score: number }) =>
      score === 0 ? removeRating(id) : setRating(id, score),
    onSuccess: (_data, { score }) => {
      qc.invalidateQueries({ queryKey: ['ratings'] })
      qc.invalidateQueries({ queryKey: ['rated-albums'] }) // my-page grid
      // The album's rating_avg / rating_count are served with the album itself,
      // so refetch those too or the displayed average goes stale.
      qc.invalidateQueries({ queryKey: ['album'] })
      qc.invalidateQueries({ queryKey: ['albums'] })
      toast(score === 0 ? '별점을 지웠습니다' : `별점 ${score / 2}점을 남겼습니다`)
    },
    onError: () => toast('별점 저장에 실패했습니다', 'error'),
  })
}
