import { apiGet, apiPost } from '@/shared/api/client'

import type { SpotifyKey } from '@/features/crawl-artist'

// 신보 체크 현황. artists = live 앨범을 보유한 체크 대상 아티스트 수,
// stale = 이번 주기(20시간) 내 아직 확인하지 않은 아티스트 수.
export interface ReleasesStatus {
  artists: number
  stale: number
}

export interface ReleaseCheckItem {
  id: string
  name: string | null
  albums: string[] // 이번에 새로 담은 앨범 이름들
}

// 한 배치의 결과. error가 있으면 서버가 배치를 중단한 것 (진행분은 저장됨).
export interface ReleaseCheckResult {
  checked: number
  new_albums: number
  enriched: number
  tracks_synced: number
  remaining: number
  artists: ReleaseCheckItem[] // 신보가 있던 아티스트만
  error?: string
}

export function fetchReleasesStatus(): Promise<ReleasesStatus> {
  return apiGet<ReleasesStatus>('/admin/spotify/releases-status')
}

// 오래 안 본 아티스트부터 최대 limit명 신보 확인 (아티스트당 Spotify 요청 1회).
// remaining이 0이 될 때까지 반복 호출하는 방식 — 트랙 동기화와 같은 계약.
export function checkReleases(key: SpotifyKey, limit = 10): Promise<ReleaseCheckResult> {
  return apiPost<ReleaseCheckResult>('/admin/spotify/check-releases', { key, limit })
}
