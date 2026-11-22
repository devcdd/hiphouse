import { chromium } from 'playwright-core'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const browser = await chromium.launch({ executablePath: exe })
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true, colorScheme: 'dark', locale: 'ko-KR' })
await ctx.addInitScript(() => localStorage.setItem('theme', 'dark'))
const page = await ctx.newPage()
await page.goto('https://hiphouse.kr/albums/2ZcI7Sbm7s2yz8l8wvIyUq', { waitUntil: 'networkidle' })
await page.waitForTimeout(1500)
const h = await page.evaluate(() => document.body.scrollHeight)
console.log('height', h)
for (let y = 844, i = 1; y < h; y += 844, i++) {
  await page.evaluate(y => window.scrollTo(0, y), y)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `shots/dark-album-scroll-${i}.png` })
}
// home with awarded filter + year 2025
await page.goto('https://hiphouse.kr/?year=2025&awarded=1', { waitUntil: 'networkidle' })
await page.waitForTimeout(1500)
await page.screenshot({ path: 'shots/dark-home-2025-awarded.png' })
await browser.close()
