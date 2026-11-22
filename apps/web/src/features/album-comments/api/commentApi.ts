import { apiGet, apiPost, apiPut, apiDelete } from '@/shared/api/client'

export interface Comment {
  id: number
  parent_id: number | null // null = 최상위 댓글
  user_id: string
  nickname: string
  avatar_url: string | null // API 기준 경로. 사진이 없거나 삭제된 댓글이면 null
  body: string // 삭제된 댓글은 빈 문자열
  created_at: string
  deleted: boolean
  edited: boolean
}

// 최상위 댓글과 앞쪽 답글 미리보기. reply_count가 replies보다 크면 나머지는 fetchReplies로 받는다.
export interface CommentThread extends Comment {
  reply_count: number
  replies: Comment[]
}

export interface CommentPage {
  threads: CommentThread[]
  next_cursor: number | null
  total: number // 답글을 포함한 살아 있는 댓글 수
}

export interface ReplyPage {
  replies: Comment[]
  next_cursor: number | null
}

export const COMMENT_PAGE_SIZE = 10

export const fetchComments = (albumId: string, before: number | null): Promise<CommentPage> =>
  apiGet<CommentPage>(`/albums/${encodeURIComponent(albumId)}/comments`, {
    limit: COMMENT_PAGE_SIZE,
    before: before ?? undefined,
  })

export const fetchReplies = (commentId: number, after: number | null): Promise<ReplyPage> =>
  apiGet<ReplyPage>(`/comments/${commentId}/replies`, { limit: COMMENT_PAGE_SIZE, after: after ?? undefined })

export const addComment = (albumId: string, body: string, parentId?: number) =>
  apiPost<Comment>(`/albums/${encodeURIComponent(albumId)}/comments`, { body, parent_id: parentId ?? null })

export const editComment = (id: number, body: string) => apiPut<Comment>(`/comments/${id}`, { body })

export const removeComment = (id: number) => apiDelete(`/comments/${id}`)
