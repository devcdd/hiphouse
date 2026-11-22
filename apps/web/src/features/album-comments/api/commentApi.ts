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

export const fetchComments = (albumId: string): Promise<Comment[]> =>
  apiGet<Comment[]>(`/albums/${encodeURIComponent(albumId)}/comments`)

export const addComment = (albumId: string, body: string, parentId?: number) =>
  apiPost<Comment>(`/albums/${encodeURIComponent(albumId)}/comments`, { body, parent_id: parentId ?? null })

export const editComment = (id: number, body: string) => apiPut<Comment>(`/comments/${id}`, { body })

export const removeComment = (id: number) => apiDelete(`/comments/${id}`)
