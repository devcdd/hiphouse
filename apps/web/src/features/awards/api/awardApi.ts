import { apiGet, apiPost, apiDelete } from '@/shared/api/client'

export type AwardKind = 'album' | 'artist'

export interface Award {
  id: number
  host: string // 주최
  name: string // 수상명
  year: number | null
}

const base = (kind: AwardKind, id: string) => `/${kind}s/${encodeURIComponent(id)}/awards`

export const fetchAwards = (kind: AwardKind, id: string) => apiGet<Award[]>(base(kind, id))

// Admin
export const addAward = (kind: AwardKind, id: string, body: Omit<Award, 'id'>) => apiPost<Award>(base(kind, id), body)
export const removeAward = (awardId: number) => apiDelete(`/awards/${awardId}`)
