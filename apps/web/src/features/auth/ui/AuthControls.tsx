import { Link } from 'react-router-dom'
import { CircleUser, Settings } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { useStartSignIn } from '@/features/auth/lib/useStartSignIn'
import styles from './AuthControls.module.css'

export function AuthControls() {
  const { isAuthed, isAdmin, user, signOut } = useAuth()
  const startSignIn = useStartSignIn()

  if (!isAuthed) {
    return (
      <button type="button" className={styles.login} onClick={startSignIn}>
        로그인
      </button>
    )
  }

  return (
    <div className={styles.wrap}>
      {isAdmin && (
        <Link to="/admin" className={styles.link} title="관리자" aria-label="관리자">
          <Settings size={17} />
        </Link>
      )}
      <Link to="/me" className={styles.me} title="마이페이지">
        <CircleUser size={17} aria-hidden />
        <span className={styles.name}>{user?.nickname || '사용자'}</span>
      </Link>
      <button type="button" className={styles.logout} onClick={signOut}>
        로그아웃
      </button>
    </div>
  )
}
