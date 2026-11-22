import { chromium } from 'playwright-core'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const browser = await chromium.launch({ executablePath: exe })
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true, colorScheme: 'dark', locale: 'ko-KR' })
await ctx.addInitScript(() => localStorage.setItem('theme', 'dark'))
const page = await ctx.newPage()
await page.goto('https://hiphouse.kr/albums/2ZcI7Sbm7s2yz8l8wvIyUq', { waitUntil: 'networkidle' })
await page.waitForTimeout(1500)
const info = await page.evaluate(() => {
  const hs = [...document.querySelectorAll('h1,h2,h3')]
  const tl = hs.find(h => h.textContent.trim() === '트랙리스트')
  const ifr = document.querySelector('iframe')
  let wrap = ifr; while (wrap && wrap.parentElement !== tl.parentElement) wrap = wrap.parentElement
  tl.style.display = 'none'; if (wrap) wrap.style.display = 'none'
  const r = [...document.querySelectorAll('*')].find(e => e.children.length === 0 && e.textContent.trim() === '4.8')
  return { ratingY: r && r.getBoundingClientRect().top + window.scrollY, h: document.body.scrollHeight }
})
console.log(info)
await page.evaluate(y => window.scrollTo(0, y), info.ratingY - 90); await page.waitForTimeout(400)
await page.screenshot({ path: 'shots/dark-album-rating.png' })
await browser.close()
