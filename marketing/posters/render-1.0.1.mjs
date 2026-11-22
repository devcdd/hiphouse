// v1.0.1 업데이트 소식: 썸네일 + 발매 기념일, 담벼락 소개 포스터. render.mjs와 같은 템플릿(poster.html)을 쓴다.
import { chromium } from 'playwright-core'
import { readFileSync } from 'node:fs'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const tpl = readFileSync('poster.html', 'utf8')
const S = './shots/'
const OUT = '1.0.1/'
const top = `<div class="top"><img class="logo" src="logo.png" alt="힙집"><div class="pill">hiphouse.kr</div></div>`
const posters = [
  { name: 'hiphouse-thumb', cls: 'thumb', body: `
    <div class="grid">${Array.from({ length: 20 }, (_, i) => `<img src="${S}covers-1.0.1/${i % 12}.jpg">`).join('')}</div>
    <div class="vignette"></div>
    <div class="kicker" style="margin-top:0">힙집 v1.0.1</div>
    <h1 style="font-size:150px;word-spacing:.15em">업데이트 소식</h1>
    <h2>발매 기념일과 담벼락이 생겼어요</h2>
    <div class="sub">같은 날 나온 지난 앨범을 다시 듣고,<br>좋아하는 아티스트에게 한마디 남겨보세요.</div>
    <div class="url">hiphouse.kr</div>` },
  { name: 'hiphouse-poster-1', body: `${top}
    <div class="kicker">01 · 발매 기념일</div>
    <h1>N년 전 오늘,<br><em>무슨 앨범이 나왔을까?</em></h1>
    <div class="sub">같은 날 발매된 지난 앨범을 매일 홈에서 보여드려요.<br>오늘 생일을 맞은 앨범을 다시 들어보세요.</div>
    <div class="tags"><span class="tag">1년 전 오늘</span><span class="tag on">3년 전 오늘</span><span class="tag">9년 전 오늘</span><span class="tag">15년 전 오늘</span></div>
    <div class="stage" style="top:640px"><div class="phone"><img src="${S}dark-home-onthisday.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-2', body: `${top}
    <div class="kicker">02 · 담벼락</div>
    <h1>좋아하는 아티스트에게<br><em>한마디 남기기</em></h1>
    <div class="sub">아티스트 페이지 담벼락에 응원 한마디를 남겨보세요.<br>내가 쓴 글은 마이페이지에서 모아볼 수 있어요.</div>
    <div class="stage" style="top:560px"><div class="phone"><img src="${S}dark-artist-wall.png"></div></div><div class="fade"></div>` },
]
const browser = await chromium.launch({ executablePath: exe })
for (const p of posters) {
  const page = await browser.newPage({ viewport: { width: 1080, height: 1350 }, deviceScaleFactor: 2 })
  const html = tpl.replace('{{cls}}', p.cls ?? '').replace('{{bodycls}}', p.bodycls ?? '').replace('{{body}}', p.body)
  await page.goto(`file://${process.cwd()}/poster.html`, { waitUntil: 'load' }) // about:blank에서는 file:// 이미지 로드가 막혀 file 오리진 먼저 확보
  await page.setContent(html.replace('<head>', `<head><base href="file://${process.cwd()}/">`), { waitUntil: 'networkidle' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `${OUT}${p.name}.png` })
  await page.close()
  console.log('ok', p.name)
}
await browser.close()
