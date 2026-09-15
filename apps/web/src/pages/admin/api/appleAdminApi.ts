import { apiGet, apiPost } from '@/shared/api/client'

// Apple Music 연동 현황. albums = UPC가 있는 live 앨범 수, linked = apple_id가
// 붙은 앨범 수, pending = 아직 Apple에 조회하지 않은 앨범 수.
export interface AppleStatus {
  configured: boolean
  albums: number
  linked: number
  pending: number
  artists_linked: number
}

// 한 배치의 결과. error가 있으면 서버가 배치를 중단한 것 (진행분은 저장됨).
export interface AppleLinkResult {
  checked: number
  linked: number
  artists_linked: number
  named: number // 한글 표시 이름이 채워진 앨범·아티스트·트랙 수
  remaining: number
  error?: string
}

export function fetchAppleStatus(): Promise<AppleStatus> {
  return apiGet<AppleStatus>('/admin/apple/status')
}

// 미조회 앨범을 최신 발매순으로 최대 limit개 Apple에 매칭 (UPC 있으면 25개당 요청 1회, 없으면 검색 폴백으로 앨범당 2회).
// remaining이 0이 될 때까지 반복 호출하는 방식 — 트랙 동기화와 같은 계약.
export function linkApple(limit = 50): Promise<AppleLinkResult> {
  return apiPost<AppleLinkResult>('/admin/apple/link', { limit })
}
