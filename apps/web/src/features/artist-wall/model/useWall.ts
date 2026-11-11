import { useMemo } from 'react'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  addWallPost,
  fetchWallCount,
  fetchWallPosts,
  fetchWallQuota,
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

// 접힌 헤더에 띄우는 글 수. 'wall' 키 아래라서 글을 쓰거나 지울 때 목록과 함께 무효화된다.
export function useWallCount(artistId: string) {
  return useQuery({ queryKey: ['wall', artistId, 'count'], queryFn: () => fetchWallCount(artistId) })
}

export function useWallQuota(artistId: string, enabled: boolean) {
  return useQuery({ queryKey: ['wall-quota', artistId], queryFn: () => fetchWallQuota(artistId), enabled })
}

export function useAddWallPost(artistId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (body: string) => addWallPost(artistId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['wall', artistId] })
      qc.invalidateQueries({ queryKey: ['wall-quota', artistId] })
    },
    onError: (e) => {
      const limited = e.message.includes('limit reached')
      if (limited) qc.invalidateQueries({ queryKey: ['wall-quota', artistId] })
      toast(limited ? '오늘은 이 담벼락에 더 남길 수 없어요' : '메시지 등록에 실패했습니다', 'error')
    },
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
