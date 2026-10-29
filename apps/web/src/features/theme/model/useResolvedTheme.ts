import { useSyncExternalStore } from 'react'

const media = () => window.matchMedia('(prefers-color-scheme: dark)')

function subscribe(onChange: () => void) {
  // useTheme은 토글마다 인스턴스별 state라 다른 컴포넌트가 못 본다 — 결과물인 data-theme을 구독한다.
  const observer = new MutationObserver(onChange)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  const mq = media()
  mq.addEventListener('change', onChange)
  return () => {
    observer.disconnect()
    mq.removeEventListener('change', onChange)
  }
}

function snapshot(): 'light' | 'dark' {
  const t = document.documentElement.dataset.theme
  if (t === 'light' || t === 'dark') return t
  return media().matches ? 'dark' : 'light'
}

export function useResolvedTheme() {
  return useSyncExternalStore(subscribe, snapshot)
}
