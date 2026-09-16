import { Mail } from 'lucide-react'
import styles from './AppFooter.module.css'

export function AppFooter() {
  return (
    <footer className={styles.footer}>
      <div className={styles.inner}>
        <span className={styles.brand}>힙집 HipHouse</span>
        <nav className={styles.links} aria-label="연락처">
          <a href="mailto:developer.cdd@gmail.com" className={styles.link}>
            <Mail size={15} aria-hidden />
            developer.cdd@gmail.com
          </a>
          <a
            href="https://www.instagram.com/hiphouse.kr/"
            target="_blank"
            rel="noopener noreferrer"
            className={styles.link}
          >
            <InstagramIcon />
            @hiphouse.kr
          </a>
        </nav>
      </div>
    </footer>
  )
}

// lucide-react 1.x는 브랜드 아이콘을 제공하지 않아 직접 그림
function InstagramIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect width="20" height="20" x="2" y="2" rx="5" ry="5" />
      <path d="M16 11.37A4 4 0 1 1 12.63 8 4 4 0 0 1 16 11.37z" />
      <line x1="17.5" x2="17.51" y1="6.5" y2="6.5" />
    </svg>
  )
}
