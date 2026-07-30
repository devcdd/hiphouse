import { apiGet } from '@/shared/api/client'

export interface AdminStats {
  albums: number
  deleted_albums: number
  tracks: number
  artists: number
  users: number
  ratings: number
  comments: number
  favorites: number
  follows: number
}

export const fetchAdminStats = (): Promise<AdminStats> => apiGet<AdminStats>('/admin/stats')
