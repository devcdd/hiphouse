import { useQuery } from '@tanstack/react-query'
import { Disc3, Heart, ListMusic, MessageSquare, Star, Trash2, UserPlus, Users, Mic2 } from 'lucide-react'
import { fetchAdminStats } from '../api/statsApi'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')

export function StatsTab() {
  const { data, isLoading, error } = useQuery({ queryKey: ['admin-stats'], queryFn: fetchAdminStats })

  if (isLoading) return <p className={styles.state}>불러오는 중…</p>
  if (error || !data) return <p className={styles.state}>불러오기 실패: {String(error)}</p>

  const cards = [
    { label: '등록된 앨범', value: data.albums, Icon: Disc3 },
    { label: '트랙', value: data.tracks, Icon: ListMusic },
    { label: '아티스트', value: data.artists, Icon: Mic2 },
    { label: '삭제된 앨범', value: data.deleted_albums, Icon: Trash2 },
    { label: '회원', value: data.users, Icon: Users },
    { label: '별점', value: data.ratings, Icon: Star },
    { label: '댓글', value: data.comments, Icon: MessageSquare },
    { label: '즐겨찾기', value: data.favorites, Icon: Heart },
    { label: '팔로우', value: data.follows, Icon: UserPlus },
  ]

  return (
    <div className={styles.statGrid}>
      {cards.map(({ label, value, Icon }) => (
        <div key={label} className={styles.statCard}>
          <Icon size={16} className={styles.statIcon} aria-hidden />
          <span className={styles.statLabel}>{label}</span>
          <strong className={styles.statValue}>{nf.format(value)}</strong>
        </div>
      ))}
    </div>
  )
}
