import { Outlet } from 'react-router-dom'
import { AppHeader } from '@/widgets/app-header'
import { AppFooter } from '@/widgets/app-footer'
import { ScrollToTop } from '@/app/ui/ScrollToTop'
import { ScrollTopButton } from '@/shared/ui/ScrollTopButton'

export function Layout() {
  return (
    <>
      <AppHeader />
      <ScrollToTop />
      <Outlet />
      <AppFooter />
      <ScrollTopButton />
    </>
  )
}
