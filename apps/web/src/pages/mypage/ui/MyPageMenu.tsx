import { Link } from 'react-router-dom'
import { ChevronRight, Pencil, Star, Heart, UserPlus, UserRound } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

// /me — landing menu that drills into each section.
const ITEMS = [
  { to: '/me/nickname', label: '닉네임 수정하기', Icon: Pencil },
  { to: '/me/rated', label: '내가 평가한 앨범', Icon: Star },
  { to: '/favorites', label: '내가 즐겨찾기 한 앨범', Icon: Heart },
  { to: '/me/following', label: '내가 팔로우한 아티스트', Icon: UserPlus },
]

export function MyPageMenu() {
  return (
    <AuthGate>
      <Menu />
    </AuthGate>
  )
}

function Menu() {
  const { user } = useAuth()
  // 공개 프로필만 user.id가 필요해 ITEMS 상수에 넣지 못한다.
  const items = user
    ? [{ to: `/users/${encodeURIComponent(user.id)}`, label: '내 공개 프로필', Icon: UserRound }, ...ITEMS]
    : ITEMS
  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{user?.nickname || '마이페이지'}</h1>
      <nav className={styles.menu}>
        {items.map(({ to, label, Icon }) => (
          <Link key={to} to={to} className={styles.menuItem}>
            <Icon size={18} className={styles.menuIcon} aria-hidden />
            <span className={styles.menuLabel}>{label}</span>
            <ChevronRight size={18} className={styles.chev} aria-hidden />
          </Link>
        ))}
      </nav>
    </div>
  )
}
