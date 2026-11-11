import { useEffect, useRef, type RefObject } from 'react'

// Calls onReach when the returned sentinel ref scrolls into view.
// Disable (enabled=false) while loading or when no more pages remain.
// root를 주면 페이지 대신 그 스크롤 박스 기준으로 본다 (담벼락처럼 안쪽에서 스크롤하는 목록).
export function useInfiniteScroll<T extends HTMLElement>(
  onReach: () => void,
  enabled: boolean,
  { root, rootMargin = '400px' }: { root?: RefObject<HTMLElement | null>; rootMargin?: string } = {},
) {
  const ref = useRef<T | null>(null)
  useEffect(() => {
    const el = ref.current
    if (!el || !enabled) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) onReach()
      },
      { root: root?.current ?? null, rootMargin },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [onReach, enabled, root, rootMargin])
  return ref
}
