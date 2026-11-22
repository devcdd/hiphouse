import type { ReactNode } from 'react'
import { useAuth } from '@/entities/session'
import { useStartSignIn } from '@/features/auth'
import styles from './MyPage.module.css'

// Shared login gate for every my-page sub-route.
export function AuthGate({ children }: { children: ReactNode }) {
  const { isAuthed } = useAuth()
  const startSignIn = useStartSignIn()
  if (isAuthed) return <>{children}</>
  return (
    <div className={styles.page}>
      <p className={styles.state}>마이페이지는 로그인 후 이용할 수 있습니다.</p>
      <button type="button" className={styles.login} onClick={startSignIn}>
        로그인
      </button>
    </div>
  )
}
