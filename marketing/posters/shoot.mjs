import { chromium } from 'playwright-core'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const browser = await chromium.launch({ executablePath: exe })
const base = 'https://hiphouse.kr'
const pages = [
  ['home', '/?type=album,ep'],
  ['home-2023', '/?year=2023&type=album,ep'],
  ['home-2025-awarded', '/?year=2025&type=album,ep&awarded=1'],
  ['album-nowitzki', '/albums/2ZcI7Sbm7s2yz8l8wvIyUq'],
  ['album-kflip', '/albums/4EPIlAjXbTNQTracKmYnI6'],
  ['artist-beenzino', '/artists/7IrDIIq3j04exsiF3Z7CPg'],
  ['artist-esens', '/artists/6a8cUmqOsXmjzq1aWKiVpH'],
  ['search', '/search?q=빈지노'],
  ['search-empty', '/search'],
  ['favorites', '/favorites'],
  ['welcome', '/welcome'],
]
for (const theme of ['dark']) {
  const ctx = await browser.newContext({
    viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true,
    colorScheme: theme, locale: 'ko-KR',
    userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1',
  })
  await ctx.addInitScript(t => localStorage.setItem('theme', t), theme)
  const page = await ctx.newPage()
  for (const [name, path] of pages) {
    try {
      await page.goto(base + path, { waitUntil: 'networkidle', timeout: 30000 })
      await page.waitForTimeout(1500)
      await page.screenshot({ path: `shots/${theme}-${name}.png` })
            console.log('ok', theme, name)
    } catch (e) { console.log('fail', theme, name, e.message.split('\n')[0]) }
  }
  await ctx.close()
}
await browser.close()
