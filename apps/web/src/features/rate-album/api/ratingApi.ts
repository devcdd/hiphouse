import { apiGet, apiPut, apiDelete } from '@/shared/api/client'
import type { Album } from '@/entities/album'

// score counts half-stars: 1..10 == 0.5..5.0 stars (mirrors the API's CHECK).
export interface Rating {
  album_id: string
  score: number
}

export const fetchRatings = (): Promise<Rating[]> => apiGet<Rating[]>('/ratings')

// 앨범 카드 + 목록 주인이 준 점수(하프스타 1..10).
export type RatedAlbum = Album & { score: number }

// 내가 평가한 앨범 전체. sort를 생략하면 최근 평가순이다.
export const fetchRatedAlbums = (sort?: string): Promise<RatedAlbum[]> =>
  apiGet<RatedAlbum[]>('/ratings/albums', { sort })
export const setRating = (albumId: string, score: number) =>
  apiPut(`/ratings/${encodeURIComponent(albumId)}`, { score })
export const removeRating = (albumId: string) => apiDelete(`/ratings/${encodeURIComponent(albumId)}`)
