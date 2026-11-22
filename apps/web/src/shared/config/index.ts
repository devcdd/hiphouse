// Dev goes through Vite proxy (/api). Override in prod via VITE_API_BASE.
export const API_BASE = import.meta.env.VITE_API_BASE ?? '/api'

// 서버가 API 기준 경로(예: avatar_url)를 줄 때 <img src>에 넣을 주소로 바꾼다.
export const apiUrl = (path: string) => API_BASE + path

// Kakao OAuth — REST API key from the Kakao developer console.
export const KAKAO_REST_KEY = import.meta.env.VITE_KAKAO_REST_API_KEY ?? ''
export const kakaoRedirectUri = () => window.location.origin + '/auth/kakao/callback'
