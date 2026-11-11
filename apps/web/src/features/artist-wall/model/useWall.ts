import { useMemo } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  addWallPost,
  fetchWallPosts,
  removeWallPost,
  WALL_PAGE_SIZE,
  type WallPost,
} from '@/features/artist-wall/api/wallApi'
import { useToast } from '@/shared/ui/toast'

const POLL_MS = 20_000

// pages[0]이 최신 페이지이고, 다음 페이지일수록 과거다. 화면에는 오래된 순으로 편다.
export function useWall(artistId: string) {
  const query = useInfiniteQuery({
    queryKey: ['wall', artistId],
    queryFn: ({ pageParam }) => fetchWallPosts(artistId, pageParam),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.length < WALL_PAGE_SIZE ? undefined : last[0]?.id),
    refetchInterval: POLL_MS, // 채팅창처럼 다른 사람 글이 올라오게. 탭이 숨겨지면 react-query가 멈춘다.
  })
  const posts = useMemo<WallPost[]>(() => {
    // 폴링으로 최신 페이지가 밀리면 다음 페이지와 겹칠 수 있어 id로 중복을 걷어낸다.
    const seen = new Set<number>()
    const out: WallPost[] = []
    for (const page of [...(query.data?.pages ?? [])].reverse()) {
      for (const p of page) {
        if (!seen.has(p.id)) {
          seen.add(p.id)
          out.push(p)
        }
      }
    }
    return out.sort((a, b) => a.id - b.id)
  }, [query.data])
  return { ...query, posts }
}

export function useAddWallPost(artistId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (body: string) => addWallPost(artistId, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['wall', artistId] }),
    onError: (e) =>
      toast(e.message.includes('too fast') ? '조금 천천히 남겨 주세요' : '메시지 등록에 실패했습니다', 'error'),
  })
}

export function useRemoveWallPost(artistId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (id: number) => removeWallPost(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['wall', artistId] }),
    onError: () => toast('메시지 삭제에 실패했습니다', 'error'),
  })
}
