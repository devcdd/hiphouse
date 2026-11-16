import { Link } from 'react-router-dom'
import { ChevronRight, Pencil, Star, Heart, MessagesSquare, UserPlus, UserRound, ShieldCheck } from 'lucide-react'
import { useAuth, useAdminMode, setAdminMode } from '@/entities/session'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

// /me — 각 섹션으로 들어가는 진입 메뉴. 공개 설정은 여기 없다. 공개 프로필 페이지에
// 붙어 있어야 "남에게 어떻게 보이나"를 그 자리에서 확인하며 끄고 켤 수 있다.
const ITEMS = [
  { to: '/me/nickname', label: '닉네임 수정하기', Icon: Pencil },
  { to: '/me/rated', label: '내가 평가한 앨범', Icon: Star },
  { to: '/favorites', label: '내가 즐겨찾기 한 앨범', Icon: Heart },
  { to: '/me/following', label: '내가 팔로우한 아티스트', Icon: UserPlus },
  { to: '/me/wall', label: '내가 쓴 담벼락 글', Icon: MessagesSquare },
]

export function MyPageMenu() {
  return (
    <AuthGate>
      <Menu />
    </AuthGate>
  )
}

function Menu() {
  const { user, isAdminUser } = useAuth()
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
        {isAdminUser && <AdminModeToggle />}
      </nav>
    </div>
  )
}

function AdminModeToggle() {
  const on = useAdminMode()
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      className={styles.menuItem}
      onClick={() => setAdminMode(!on)}
    >
      <ShieldCheck size={18} className={styles.menuIcon} aria-hidden />
      <span className={styles.menuLabel}>
        관리자 모드
        <span className={styles.menuHint}>끄면 관리 도구를 숨기고 일반 사용자 화면으로 봅니다</span>
      </span>
      <span className={on ? `${styles.switch} ${styles.switchOn}` : styles.switch} aria-hidden />
    </button>
  )
}
