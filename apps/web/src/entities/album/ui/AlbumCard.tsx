import { Fragment } from 'react'
import { Link } from 'react-router-dom'
import { Star } from 'lucide-react'
import type { Album } from '@/entities/album/model/types'
import { displayName } from '@/shared/lib/displayName'
import { awardHostLabel } from '@/shared/lib/awardHosts'
import styles from './AlbumCard.module.css'

export function AlbumCard({ album }: { album: Album }) {
  const albumHref = `/albums/${encodeURIComponent(album.id)}`
  const title = displayName(album)
  // Each credited artist carries its own Spotify id, so every name links to that artist.
  const artistNames = album.artists.map(displayName).join(', ')

  return (
    <article className={styles.card}>
      <Link to={albumHref} className={styles.art} aria-label={title}>
        {album.image_url ? (
          <img src={album.image_url} alt={title} loading="lazy" />
        ) : (
          <div className={styles.placeholder}>{title.slice(0, 1)}</div>
        )}
      </Link>
      <div className={styles.meta}>
        <div className={styles.titleRow}>
          <Link to={albumHref} className={styles.name} title={title}>
            {title}
          </Link>
          {album.type_label && <span className={styles.badge}>{album.type_label}</span>}
        </div>
        <div className={styles.artist} title={artistNames}>
          {album.artists.map((a, i) => (
            <Fragment key={a.id}>
              {i > 0 && <span className={styles.sep}>, </span>}
              <Link to={`/artists/${encodeURIComponent(a.id)}`} className={styles.artistLink}>
                {displayName(a)}
              </Link>
            </Fragment>
          ))}
        </div>
        <div className={styles.footer}>
          {(album.release_date ?? album.year) != null && (
            <span className={styles.year}>{album.release_date ?? album.year}</span>
          )}
          {/* Makes 인기순 / 별점 높은 순 legible — hidden until someone rates it.
              The count in parens is COMMENTS, not raters — rating_count stays off-card. */}
          {album.rating_count > 0 && album.rating_avg != null && (
            <span className={styles.rating} title={`평균 별점 ${album.rating_avg.toFixed(1)} · 댓글 ${album.comment_count}개`}>
              <Star size={12} strokeWidth={2.2} fill="currentColor" />
              {album.rating_avg.toFixed(1)}
              {album.comment_count > 0 && <span className={styles.count}>({album.comment_count})</span>}
            </span>
          )}
        </div>
        {album.awards.length > 0 && (
          <div className={styles.awards}>
            {album.awards.map((a) => (
              <span
                key={a.id}
                className={styles.award}
                title={`${awardHostLabel(a.host)}${a.year != null ? ` · ${a.year}` : ''}`}
              >
                {a.name}
              </span>
            ))}
          </div>
        )}
      </div>
    </article>
  )
}
