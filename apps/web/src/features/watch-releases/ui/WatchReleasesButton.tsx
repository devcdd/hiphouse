import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Bell, BellOff } from 'lucide-react'
import { updateArtistReleasesWatch } from '@/entities/artist'
import { useAuth } from '@/entities/session'
import styles from './WatchReleasesButton.module.css'

// 관리자 전용 신보 감시 토글. 신보 체크 탭은 이 플래그가 켜진 아티스트만
// 확인한다 — 콜라보 싱글로 딸려 들어온 비힙합 아티스트를 대상에서 빼는 스위치.
// 관리자가 아니면 아무것도 렌더하지 않는다.
export function WatchReleasesButton({ artistId, watching }: { artistId: string; watching: boolean }) {
  const { isAdmin } = useAuth()
  const qc = useQueryClient()

  const toggle = useMutation({
    mutationFn: () => updateArtistReleasesWatch(artistId, !watching),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['artist', artistId] })
      qc.invalidateQueries({ queryKey: ['releases-status'] })
    },
  })

  if (!isAdmin) return null

  return (
    <button
      type="button"
      className={watching ? `${styles.btn} ${styles.on}` : styles.btn}
      disabled={toggle.isPending}
      onClick={() => toggle.mutate()}
      title="신보 체크 탭이 확인하는 대상인지 여부"
    >
      {watching ? <Bell size={14} strokeWidth={2.2} /> : <BellOff size={14} strokeWidth={2.2} />}
      {toggle.isPending ? '저장 중…' : watching ? '신보 감시 중' : '신보 감시 꺼짐'}
    </button>
  )
}
