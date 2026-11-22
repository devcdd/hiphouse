import { useEffect } from 'react'

const DEFAULT_TITLE = '힙집 HipHouse'

// 탭 제목과 JS로 렌더링하는 검색봇이 읽는 제목. title이 없으면(데이터 로딩 중 등) 기본 제목을 쓴다.
// 화면을 떠날 때 기본 제목으로 되돌려야 다음 화면이 제목을 안 정해도 이전 제목이 남지 않는다.
export function useDocumentTitle(title: string | null | undefined) {
  useEffect(() => {
    document.title = title ? `${title} | 힙집` : DEFAULT_TITLE
    return () => {
      document.title = DEFAULT_TITLE
    }
  }, [title])
}
