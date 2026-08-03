import { Routes, Route } from 'react-router-dom'
import { Layout } from '@/app/ui/Layout'
import { AlbumsPage } from '@/pages/albums'
import { AlbumDetailPage } from '@/pages/album-detail'
import { ArtistDetailPage } from '@/pages/artist-detail'
import { SearchPage } from '@/pages/search'
import { AuthCallbackPage } from '@/pages/auth-callback'
import { FavoritesPage } from '@/pages/favorites'
import { FollowedArtistsPage, MyPageMenu, NicknamePage, RatedAlbumsPage } from '@/pages/mypage'
import { UserProfilePage } from '@/pages/user-profile'
import { AdminPage } from '@/pages/admin'
import { WelcomePage } from '@/pages/welcome'

export function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<AlbumsPage />} />
        <Route path="/albums/:id" element={<AlbumDetailPage />} />
        <Route path="/artists/:id" element={<ArtistDetailPage />} />
        <Route path="/users/:id" element={<UserProfilePage />} />
        <Route path="/search" element={<SearchPage />} />
        <Route path="/favorites" element={<FavoritesPage />} />
        <Route path="/welcome" element={<WelcomePage />} />
        <Route path="/me" element={<MyPageMenu />} />
        <Route path="/me/nickname" element={<NicknamePage />} />
        <Route path="/me/rated" element={<RatedAlbumsPage />} />
        <Route path="/me/following" element={<FollowedArtistsPage />} />
        <Route path="/admin" element={<AdminPage />} />
        <Route path="/auth/kakao/callback" element={<AuthCallbackPage />} />
      </Route>
    </Routes>
  )
}
