import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BadgeCheck, X } from 'lucide-react'
import { ArtistCombobox, type Artist } from '@/entities/artist'
import { Avatar } from '@/shared/ui/Avatar'
import { useToast } from '@/shared/ui/toast'
import { displayName } from '@/shared/lib/displayName'
import {
  CLAIM_NOTE_MAX,
  claimArtistName,
  fetchMyClaim,
  submitMyClaim,
  withdrawMyClaim,
  type ClaimStatus,
} from '@/features/artist-claim/api/claimApi'
import styles from './ArtistClaimSection.module.css'

type Picked = Pick<Artist, 'id' | 'name' | 'display_name' | 'image_url'>

const STATUS_LABEL: Record<ClaimStatus, string> = {
  pending: '검수 대기',
  approved: '인증됨',
  rejected: '반려됨',
}

// "아티스트 본인입니다" 신청. 관리자가 승인하면 담벼락에서 이름이 아티스트명으로 강조된다.
export function ArtistClaimSection() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data: claim, isLoading } = useQuery({ queryKey: ['my-artist-claim'], queryFn: fetchMyClaim })
  const [checked, setChecked] = useState(false)
  const [editing, setEditing] = useState(false)
  const [picked, setPicked] = useState<Picked | null>(null)
  const [note, setNote] = useState('')

  const done = () => {
    setEditing(false)
    qc.invalidateQueries({ queryKey: ['my-artist-claim'] })
    qc.invalidateQueries({ queryKey: ['wall'] }) // 철회하면 담벼락 표시도 바로 바뀐다
  }
  const submit = useMutation({
    mutationFn: () => submitMyClaim(picked!.id, note.trim()),
    onSuccess: () => {
      done()
      toast('인증을 요청했습니다. 관리자 검수 후 반영됩니다')
    },
    onError: () => toast('인증 요청에 실패했습니다', 'error'),
  })
  const withdraw = useMutation({
    mutationFn: withdrawMyClaim,
    onSuccess: () => {
      setChecked(false)
      done()
      toast('아티스트 인증을 철회했습니다')
    },
    onError: () => toast('철회에 실패했습니다', 'error'),
  })

  if (isLoading) return null

  const openForm = () => {
    setPicked(
      claim
        ? {
            id: claim.artist_id,
            name: claim.artist_name,
            display_name: claim.artist_display_name,
            image_url: claim.artist_image_url,
          }
        : null,
    )
    setNote(claim?.note ?? '')
    setEditing(true)
  }
  const formOpen = editing || (!claim && checked)

  return (
    <section className={styles.card}>
      <label className={styles.checkRow}>
        <input
          type="checkbox"
          checked={!!claim || checked}
          disabled={withdraw.isPending}
          onChange={(e) => {
            if (e.target.checked) {
              setChecked(true)
              openForm()
            } else if (claim) {
              if (window.confirm('아티스트 인증 신청을 철회할까요? 인증된 상태였다면 표시도 사라집니다.')) withdraw.mutate()
            } else {
              setChecked(false)
              setEditing(false)
            }
          }}
        />
        아티스트 본인입니다
      </label>
      <p className={styles.help}>
        관리자가 확인한 뒤 실제 아티스트와 연결하면, 담벼락에 글을 쓸 때 이름이 아티스트명으로 강조됩니다.
      </p>

      {claim && !editing && (
        <div className={styles.status}>
          <Avatar src={claim.artist_image_url} name={claimArtistName(claim)} size={32} />
          <Link to={`/artists/${encodeURIComponent(claim.artist_id)}`} className={styles.artistName}>
            {claimArtistName(claim)}
          </Link>
          <span className={`${styles.badge} ${styles[claim.status]}`}>
            {claim.status === 'approved' && <BadgeCheck size={13} strokeWidth={2.4} aria-hidden />}
            {STATUS_LABEL[claim.status]}
          </span>
          <button type="button" className={styles.ghost} onClick={openForm}>
            {claim.status === 'rejected' ? '다시 신청' : '변경'}
          </button>
        </div>
      )}

      {formOpen && (
        <form
          className={styles.form}
          onSubmit={(e) => {
            e.preventDefault()
            if (picked && !submit.isPending) submit.mutate()
          }}
        >
          {picked ? (
            <div className={styles.picked}>
              <Avatar src={picked.image_url} name={displayName(picked)} size={28} />
              <span className={styles.artistName}>{displayName(picked)}</span>
              <button type="button" className={styles.unpick} onClick={() => setPicked(null)} aria-label="다른 아티스트 고르기">
                <X size={14} strokeWidth={2.4} />
              </button>
            </div>
          ) : (
            <ArtistCombobox autoFocus onPick={setPicked} placeholder="본인 아티스트 검색" />
          )}
          <textarea
            className={styles.note}
            value={note}
            maxLength={CLAIM_NOTE_MAX}
            rows={2}
            onChange={(e) => setNote(e.target.value)}
            placeholder="확인용 메모 (예: 공식 인스타그램 계정, 연락 가능한 채널)"
            aria-label="확인용 메모"
          />
          {claim?.status === 'approved' && (
            <p className={styles.warn}>저장하면 다시 검수 대기로 바뀌고, 승인 전까지 인증 표시가 빠집니다.</p>
          )}
          <div className={styles.actions}>
            {claim && (
              <button type="button" className={styles.ghost} onClick={() => setEditing(false)}>
                취소
              </button>
            )}
            <button type="submit" className={styles.submit} disabled={!picked || submit.isPending}>
              {submit.isPending ? '요청 중…' : '검수 요청'}
            </button>
          </div>
        </form>
      )}
    </section>
  )
}
