import { Fragment } from 'react'
import { useAlbumTracks } from '@/entities/album'
import { EditDisplayNameButton } from '@/features/edit-display-name'
import { displayName } from '@/shared/lib/displayName'
import styles from './AlbumDetailPage.module.css'

// 207959ms → "3:28"
const fmtDuration = (ms: number) => {
  const s = Math.round(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// 접힌 <details> 안에서 렌더 — open이 처음 true가 될 때 목록을 불러온다.
export function TrackList({ albumId, open }: { albumId: string; open: boolean }) {
  const { data: tracks, isLoading, error } = useAlbumTracks(albumId, open)

  if (isLoading) return <p className={styles.state}>불러오는 중…</p>
  if (error) return <p className={styles.state}>불러오기 실패: {String(error)}</p>
  if (!tracks) return null
  if (tracks.length === 0) return <p className={styles.state}>아직 트랙 정보가 없습니다</p>

  const multiDisc = tracks.some((t) => t.disc_number > 1)
  return (
    <ol className={styles.trackList}>
      {tracks.map((t, i) => (
        <Fragment key={t.id}>
          {multiDisc && (i === 0 || tracks[i - 1].disc_number !== t.disc_number) && (
            <li className={styles.discHeader}>Disc {t.disc_number}</li>
          )}
          <li className={styles.trackRow}>
            <span className={styles.trackNo}>{t.track_number}</span>
            <span className={styles.trackName}>
              {t.spotify_url ? (
                <a href={t.spotify_url} target="_blank" rel="noreferrer">
                  {displayName(t)}
                </a>
              ) : (
                displayName(t)
              )}
              {t.explicit && (
                <span className={styles.explicit} title="Explicit">
                  E
                </span>
              )}
            </span>
            <span className={styles.trackArtists}>{t.artists.map((a) => a.name).join(', ')}</span>
            {t.duration_ms != null && <span className={styles.trackDuration}>{fmtDuration(t.duration_ms)}</span>}
            <EditDisplayNameButton kind="track" albumId={albumId} id={t.id} name={t.name} displayName={t.display_name} />
          </li>
        </Fragment>
      ))}
    </ol>
  )
}
