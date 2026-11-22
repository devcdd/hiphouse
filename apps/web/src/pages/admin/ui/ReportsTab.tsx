import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Flag, Trash2 } from 'lucide-react'
import { AlbumCard } from '@/entities/album'
import { clearFlags, fetchFlaggedAlbums } from '@/features/report-album'
import { apiDelete } from '@/shared/api/client'
import { displayName } from '@/shared/lib/displayName'
import { useToast } from '@/shared/ui/toast'
import styles from './AdminPage.module.css'

// 사용자가 "힙합이 아니에요"로 신고한 앨범 — 신고 많은 순. 여기서 바로
// 완전 삭제하거나, 확인 결과 문제없으면 신고를 지운다.
export function ReportsTab() {
  const qc = useQueryClient()
  const toast = useToast()
  const {
    data: albums,
    isLoading,
    error,
  } = useQuery({ queryKey: ['admin-flags', 'not-hiphop'], queryFn: () => fetchFlaggedAlbums('not-hiphop') })

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ['admin-flags', 'not-hiphop'] })
    qc.invalidateQueries({ queryKey: ['albums'] })
  }

  const remove = useMutation({
    mutationFn: (id: string) => apiDelete(`/albums/${encodeURIComponent(id)}`),
    onSuccess: () => {
      refresh()
      toast('앨범을 삭제했습니다')
    },
    onError: () => toast('처리에 실패했습니다', 'error'),
  })
  const confirmRemove = (id: string, name: string) => {
    const msg = `“${name}”을(를) 완전히 삭제할까요?\n별점, 댓글, 즐겨찾기도 함께 지워지고 되돌릴 수 없습니다.`
    if (window.confirm(msg)) remove.mutate(id)
  }

  const dismiss = useMutation({
    mutationFn: (id: string) => clearFlags('not-hiphop', id),
    onSuccess: () => {
      refresh()
      toast('신고를 정리했습니다')
    },
    onError: () => toast('처리에 실패했습니다', 'error'),
  })

  if (isLoading) return <p className={styles.state}>불러오는 중…</p>
  if (error) return <p className={styles.state}>불러오기 실패: {String(error)}</p>
  if ((albums?.length ?? 0) === 0) return <p className={styles.state}>신고된 앨범이 없습니다.</p>

  return (
    <div className={styles.grid}>
      {albums?.map((a) => (
        <div key={a.id} className={a.deleted_at ? `${styles.item} ${styles.dimmed}` : styles.item}>
          <span className={styles.reportCount} title={`${a.report_count}명이 신고`}>
            <Flag size={12} strokeWidth={2.4} fill="currentColor" />
            {a.report_count}
          </span>
          <AlbumCard album={a} />
          <div className={styles.reportActions}>
            <button
              type="button"
              className={styles.reportDelete}
              disabled={remove.isPending}
              onClick={() => confirmRemove(a.id, displayName(a))}
            >
              <Trash2 size={13} strokeWidth={2.4} />
              삭제
            </button>
            <button
              type="button"
              className={styles.reportDismiss}
              disabled={dismiss.isPending}
              onClick={() => dismiss.mutate(a.id)}
            >
              <Check size={13} strokeWidth={2.4} />
              문제없음
            </button>
          </div>
        </div>
      ))}
    </div>
  )
}
