import { useParams } from 'react-router-dom'
import { useArtist } from '@/entities/artist'
import { useAuth } from '@/entities/session'
import { AlbumFeed } from '@/widgets/album-feed'
import { FollowButton } from '@/features/follow-artist'
import { RefreshAlbumsButton } from '@/features/crawl-artist'
import { EditDisplayNameButton } from '@/features/edit-display-name'
import { WatchReleasesButton } from '@/features/watch-releases'
import { Avatar } from '@/shared/ui/Avatar'
import { displayName } from '@/shared/lib/displayName'
import styles from './ArtistDetailPage.module.css'

export function ArtistDetailPage() {
  const { id = '' } = useParams()
  const { data: artist, isLoading, error } = useArtist(id)
  const { isAdmin } = useAuth()
  const name = artist ? displayName(artist) : id

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Avatar src={artist?.image_url} name={name} size={88} shape="square" />
        <div className={styles.headerInfo}>
          <h1 className={styles.name}>{isLoading ? '불러오는 중…' : name}</h1>
          <div className={styles.headerActions}>{artist && <FollowButton artistId={artist.id} />}</div>
        </div>
        {artist?.follower_count != null && (
          <div className={styles.followers}>
            <span className={styles.followersCount}>{artist.follower_count.toLocaleString()}</span>
            <span className={styles.followersLabel}>팔로워</span>
          </div>
        )}
      </header>

      {/* 관리자 도구는 헤더 아래 한 줄로 — 팔로우 버튼과 섞이면 좁은 화면에서 줄이 터진다.
          두 컴포넌트 모두 관리자가 아니면 아무것도 렌더하지 않는다. */}
      {artist && isAdmin && (
        <div className={styles.adminBar}>
          <span className={styles.adminTag}>관리자</span>
          <RefreshAlbumsButton artistId={artist.id} />
          <EditDisplayNameButton
            kind="artist"
            id={artist.id}
            name={artist.name ?? artist.id}
            displayName={artist.display_name}
          />
          <WatchReleasesButton artistId={artist.id} watching={artist.releases_watch} />
        </div>
      )}

      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      <AlbumFeed params={{ artistId: id }} />
    </div>
  )
}
