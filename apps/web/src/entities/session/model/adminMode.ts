import { useSyncExternalStore } from 'react'

// 관리자가 일반 사용자 화면을 그대로 보려고 관리 도구를 끄는 스위치. 기기별 설정이라 서버에는 저장하지 않는다.
const KEY = 'hiphouse:admin-mode'
const CHANGE_EVENT = 'hiphouse:admin-mode-change'

function read(): boolean {
  try {
    return localStorage.getItem(KEY) !== 'off'
  } catch {
    return true
  }
}

function subscribe(onChange: () => void) {
  window.addEventListener(CHANGE_EVENT, onChange)
  window.addEventListener('storage', onChange) // 다른 탭에서 바꾼 값도 따라간다
  return () => {
    window.removeEventListener(CHANGE_EVENT, onChange)
    window.removeEventListener('storage', onChange)
  }
}

export function setAdminMode(on: boolean) {
  try {
    if (on) localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, 'off')
  } catch {
    /* 저장이 막힌 환경이면 이번 탭에서만 유지되지 않는다 */
  }
  window.dispatchEvent(new Event(CHANGE_EVENT))
}

export const useAdminMode = () => useSyncExternalStore(subscribe, read, () => true)
