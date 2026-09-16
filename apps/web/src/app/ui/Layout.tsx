import { Outlet } from 'react-router-dom'
import { AppHeader } from '@/widgets/app-header'
import { AppFooter } from '@/widgets/app-footer'
import { ScrollToTop } from '@/app/ui/ScrollToTop'
import { ScrollTopButton } from '@/shared/ui/ScrollTopButton'
import styles from './Layout.module.css'

export function Layout() {
  return (
    <>
      <AppHeader />
      <ScrollToTop />
      {/* flex 컬럼 안에서 margin:auto 페이지 컨테이너가 stretch를 잃고 max-content 폭으로 튀지 않도록 block main으로 감쌈 */}
      <main className={styles.main}>
        <Outlet />
      </main>
      <AppFooter />
      <ScrollTopButton />
    </>
  )
}
