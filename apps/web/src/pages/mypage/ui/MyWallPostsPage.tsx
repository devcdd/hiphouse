import { Link } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { MyWallPostList } from '@/features/artist-wall'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

export function MyWallPostsPage() {
  return (
    <AuthGate>
      <div className={styles.page}>
        <Link to="/me" className={styles.back}>
          <ChevronLeft size={16} aria-hidden />
          마이페이지
        </Link>
        <h1 className={styles.title}>내가 쓴 담벼락 글</h1>
        <MyWallPostList />
      </div>
    </AuthGate>
  )
}
