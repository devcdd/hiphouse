import { chromium } from 'playwright-core'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const browser = await chromium.launch({ executablePath: exe })
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true, colorScheme: 'dark', locale: 'ko-KR' })
await ctx.addInitScript(() => localStorage.setItem('theme', 'dark'))
const page = await ctx.newPage()
await page.goto('https://hiphouse.kr/albums/4EPIlAjXbTNQTracKmYnI6', { waitUntil: 'networkidle' })
await page.waitForTimeout(2000)
await page.screenshot({ path: 'shots/dark-album-kflip.png' })
const y = await page.evaluate(() => {
  const h = [...document.querySelectorAll('h1,h2,h3')].find(e => e.textContent.trim() === '트랙리스트')
  return h.getBoundingClientRect().top + window.scrollY
})
await page.evaluate(y => window.scrollTo(0, y - 70), y); await page.waitForTimeout(800)
await page.screenshot({ path: 'shots/dark-album-kflip-player.png' })
await browser.close()
