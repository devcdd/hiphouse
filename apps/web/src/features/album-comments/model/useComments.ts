import { useMemo } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchComments,
  fetchReplies,
  addComment,
  editComment,
  removeComment,
} from '@/features/album-comments/api/commentApi'
import { useToast } from '@/shared/ui/toast'

export function useComments(albumId: string) {
  const { data, ...rest } = useInfiniteQuery({
    queryKey: ['comments', albumId],
    queryFn: ({ pageParam }) => fetchComments(albumId, pageParam),
    initialPageParam: null as number | null,
    getNextPageParam: (last) => last.next_cursor,
  })
  const threads = useMemo(() => data?.pages.flatMap((p) => p.threads) ?? [], [data])
  return { threads, count: data?.pages[0]?.total ?? 0, ...rest }
}

// 미리보기 뒤의 답글. 키에 after를 넣어서 미리보기가 바뀌면(답글 삭제 등) 새 커서로 다시 받는다.
// ['comments', albumId] 아래에 두어 댓글 변경 시 같이 무효화된다.
export function useMoreReplies(albumId: string, commentId: number, after: number | null, enabled: boolean) {
  const { data, ...rest } = useInfiniteQuery({
    queryKey: ['comments', albumId, 'replies', commentId, after],
    queryFn: ({ pageParam }) => fetchReplies(commentId, pageParam),
    initialPageParam: after,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
  const replies = useMemo(() => data?.pages.flatMap((p) => p.replies) ?? [], [data])
  return { replies, ...rest }
}

export function useAddComment(albumId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ body, parentId }: { body: string; parentId?: number }) => addComment(albumId, body, parentId),
    onSuccess: (_data, { parentId }) => {
      qc.invalidateQueries({ queryKey: ['comments', albumId] })
      toast(parentId ? '답글을 남겼습니다' : '댓글을 남겼습니다')
    },
    onError: () => toast('댓글 등록에 실패했습니다', 'error'),
  })
}

export function useEditComment(albumId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: string }) => editComment(id, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['comments', albumId] })
      toast('댓글을 수정했습니다')
    },
    onError: () => toast('댓글 수정에 실패했습니다', 'error'),
  })
}

export function useRemoveComment(albumId: string) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: (id: number) => removeComment(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['comments', albumId] })
      toast('댓글을 삭제했습니다')
    },
    onError: () => toast('댓글 삭제에 실패했습니다', 'error'),
  })
}
