import { Flag, Languages } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { useStartSignIn } from '@/features/auth'
import { useMyFlags, useToggleFlag } from '@/features/report-album/model/useReports'
import type { FlagKind } from '@/features/report-album/api/reportApi'
import styles from './ReportButton.module.css'

// 크롤러가 아티스트 디스코그래피를 통째로 가져오다 보니 OST 같은 게 섞이고,
// 앨범명은 Spotify가 준 영문 그대로다. 사용자가 눌러 쌓아두면 관리자가 모아 보고
// 정리한다 (관리자 페이지의 "힙합 아님 신고" / "이름 변경 요청" 탭).
const KINDS: Record<FlagKind, { icon: typeof Flag; idle: string; done: string }> = {
  'not-hiphop': { icon: Flag, idle: '힙합이 아니에요', done: '신고함' },
  rename: { icon: Languages, idle: '앨범명 좀 바꿔주세요', done: '요청함' },
}

export function ReportButton({ albumId, kind = 'not-hiphop' }: { albumId: string; kind?: FlagKind }) {
  const { isAuthed } = useAuth()
  const startSignIn = useStartSignIn()
  const flags = useMyFlags(kind, isAuthed)
  const toggle = useToggleFlag(kind)
  const { icon: Icon, idle, done } = KINDS[kind]

  if (!isAuthed) {
    return (
      <button type="button" className={styles.btn} onClick={startSignIn}>
        <Icon size={14} strokeWidth={2.2} />
        {idle}
      </button>
    )
  }

  const flagged = flags.has(albumId)
  return (
    <button
      type="button"
      className={flagged ? `${styles.btn} ${styles.on}` : styles.btn}
      aria-pressed={flagged}
      disabled={toggle.isPending}
      onClick={() => toggle.mutate({ id: albumId, flagged })}
    >
      {/* Languages 아이콘은 선으로만 그려져 있어 채우면 뭉개진다 — 깃발만 채운다. */}
      <Icon size={14} strokeWidth={2.2} fill={flagged && kind === 'not-hiphop' ? 'currentColor' : 'none'} />
      {flagged ? done : idle}
    </button>
  )
}
