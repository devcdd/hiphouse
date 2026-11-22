import { Navigate, Outlet } from 'react-router-dom'
import { AppHeader } from '@/widgets/app-header'
import { AppFooter } from '@/widgets/app-footer'
import { useNicknameRedirect } from '@/features/auth'
import { ScrollToTop } from '@/app/ui/ScrollToTop'
import { ScrollTopButton } from '@/shared/ui/ScrollTopButton'
import styles from './Layout.module.css'

export function Layout() {
  const nicknameRedirect = useNicknameRedirect()
  return (
    <>
      <AppHeader />
      <ScrollToTop />
      {/* flex 컬럼 안에서 margin:auto 페이지 컨테이너가 stretch를 잃고 max-content 폭으로 튀지 않도록 block main으로 감쌈 */}
      <main className={styles.main}>
        {/* 페이지 대신 Navigate를 그려야, 페이지 안의 다른 Navigate가 /welcome 이동을 덮어쓰지 못한다. */}
        {nicknameRedirect ? <Navigate to={nicknameRedirect} replace /> : <Outlet />}
      </main>
      <AppFooter />
      <ScrollTopButton />
    </>
  )
}
