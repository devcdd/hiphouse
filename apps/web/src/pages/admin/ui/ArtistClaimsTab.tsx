import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArtistCombobox, type Artist } from '@/entities/artist'
import { claimArtistName, fetchClaims, reviewClaim, type ArtistClaim, type ClaimStatus } from '@/features/artist-claim'
import { Avatar } from '@/shared/ui/Avatar'
import { useToast } from '@/shared/ui/toast'
import { displayName } from '@/shared/lib/displayName'
import styles from './AdminPage.module.css'

const df = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' })
const FILTERS: { key: ClaimStatus; label: string }[] = [
  { key: 'pending', label: '검수 대기' },
  { key: 'approved', label: '승인됨' },
  { key: 'rejected', label: '반려됨' },
]

// "아티스트 본인입니다" 신청 검수. 승인하면 담벼락에서 그 사용자가 아티스트로 표시된다.
export function ArtistClaimsTab() {
  const [status, setStatus] = useState<ClaimStatus>('pending')
  const { data, isLoading, error } = useQuery({
    queryKey: ['admin-artist-claims', status],
    queryFn: () => fetchClaims(status),
  })
  const claims = data ?? []

  return (
    <div className={styles.aliasWrap}>
      <div className={styles.claimFilters} role="tablist">
        {FILTERS.map((f) => (
          <button
            key={f.key}
            type="button"
            role="tab"
            aria-selected={f.key === status}
            className={f.key === status ? `${styles.claimFilter} ${styles.claimFilterActive}` : styles.claimFilter}
            onClick={() => setStatus(f.key)}
          >
            {f.label}
          </button>
        ))}
      </div>

      {isLoading && <p className={styles.state}>불러오는 중…</p>}
      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      {!isLoading && !error && claims.length === 0 && <p className={styles.state}>신청이 없습니다.</p>}

      <div className={styles.rows}>
        {claims.map((c) => (
          <ClaimRow key={c.user_id} claim={c} />
        ))}
      </div>
    </div>
  )
}

function ClaimRow({ claim }: { claim: ArtistClaim }) {
  const qc = useQueryClient()
  const toast = useToast()
  // 신청자가 고른 행이 틀렸으면 (Spotify·Apple 중복 행 등) 다른 아티스트로 바꿔 승인한다.
  const [override, setOverride] = useState<Artist | null>(null)
  const [picking, setPicking] = useState(false)

  const review = useMutation({
    mutationFn: (s: 'approved' | 'rejected') => reviewClaim(claim.user_id, s, override?.id),
    onSuccess: (_d, s) => {
      qc.invalidateQueries({ queryKey: ['admin-artist-claims'] })
      qc.invalidateQueries({ queryKey: ['wall'] })
      toast(s === 'approved' ? '승인했습니다' : '반려했습니다')
    },
    onError: () => toast('처리에 실패했습니다', 'error'),
  })

  const artistId = override?.id ?? claim.artist_id
  const artistLabel = override ? displayName(override) : claimArtistName(claim)

  return (
    <div className={`${styles.row} ${styles.claimRow}`}>
      <div className={styles.claimMain}>
        <Link to={`/users/${encodeURIComponent(claim.user_id)}`} className={styles.claimUser}>
          {claim.nickname || '(닉네임 없음)'}
        </Link>
        <span className={styles.rowId}>{df.format(new Date(claim.created_at))} 신청</span>
        <span className={styles.claimArrow}>→</span>
        <Link to={`/artists/${encodeURIComponent(artistId)}`} className={styles.claimArtist}>
          <Avatar src={override?.image_url ?? claim.artist_image_url} name={artistLabel} size={28} />
          {artistLabel}
        </Link>
        {override && <span className={styles.rowId}>(신청: {claimArtistName(claim)})</span>}
      </div>
      {claim.note && <p className={styles.claimNote}>{claim.note}</p>}
      {picking && (
        <ArtistCombobox
          autoFocus
          placeholder="매핑할 아티스트 검색"
          onPick={(a) => {
            setOverride(a)
            setPicking(false)
          }}
        />
      )}
      <div className={styles.claimActions}>
        <button type="button" className={styles.claimGhost} onClick={() => setPicking((v) => !v)}>
          {picking ? '닫기' : '아티스트 변경'}
        </button>
        {claim.status !== 'rejected' && (
          <button
            type="button"
            className={styles.claimGhost}
            disabled={review.isPending}
            onClick={() => review.mutate('rejected')}
          >
            {claim.status === 'approved' ? '인증 취소' : '반려'}
          </button>
        )}
        {(claim.status !== 'approved' || override) && (
          <button
            type="button"
            className={styles.save}
            disabled={review.isPending}
            onClick={() => review.mutate('approved')}
          >
            승인
          </button>
        )}
      </div>
    </div>
  )
}
