// v1.0.1 업데이트 소식: 썸네일 + 발매 기념일, 담벼락 소개 포스터.
// 앨범 홍보 포스터와 같은 톤으로 맞춘 update.html을 쓴다. 1.0.0 포스터(render.mjs)는 예전 poster.html을 그대로 쓴다.
// 설명 문장은 포스터에 넣지 않고 게시물 본문에 쓴다. 포스터에는 기능 이름과 한 줄 제목만 둔다.
import { chromium } from 'playwright-core'
import { readFileSync } from 'node:fs'
const exe =
  process.env.CHROME ??
  process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const tpl = readFileSync('update.html', 'utf8')
const S = './shots/'
const OUT = '1.0.1/'
const VERSION = 'V1.0.1'
const COVER = 908

const meta = (...parts) => `<div class="meta">${parts.join('  /  ')}</div>`
const sticker = (label, sub, cls = '') => `<div class="sticker ${cls}"><b>${label}</b><small>${sub}</small></div>`

// 1170px 폭 모바일 캡처에서 (x, y)부터 w×w 정사각형을 잘라 재킷 크기에 채운다.
const shot = (file, x, y, w) => {
  const k = COVER / w
  return `<div class="jacket"><img src="${S}${file}" style="width:${1170 * k}px;left:${-x * k}px;top:${-y * k}px"></div>`
}

const lp = (i, left, top, deg) =>
  `<div class="lp" style="left:${left}px;top:${top}px;transform:rotate(${deg}deg)"><img src="${S}covers-1.0.1/${i}.jpg"></div>`

const posters = [
  {
    name: 'hiphouse-thumb',
    cls: 'ink',
    title: 120,
    body: `
      <div class="floor">${lp(3, -6, 30, -7)}${lp(7, 444, 6, 5)}${lp(0, 214, 400, -2)}</div>
      ${sticker('UPDATE', VERSION)}
      <div class="caption">
        <div class="label">발매 기념일 · 담벼락</div>
        <h1>업데이트 소식</h1>
        ${meta(VERSION, '2026.11', 'HIPHOUSE.KR')}
      </div>`,
  },
  {
    name: 'hiphouse-poster-1',
    cls: 'forest',
    title: 66,
    body: `
      ${shot('dark-home-onthisday.png', 73, 440, 1024)}
      ${sticker('NEW', '01  ANNIVERSARY')}
      <div class="caption">
        <div class="label">01 발매 기념일</div>
        <h1>N년 전 오늘,<br>무슨 앨범이 나왔을까?</h1>
        ${meta(VERSION, '01 / 02', 'HIPHOUSE.KR')}
      </div>`,
  },
  {
    name: 'hiphouse-poster-2',
    cls: 'burgundy',
    title: 66,
    body: `
      ${shot('dark-artist-wall.png', 46, 338, 1078)}
      ${sticker('NEW', '02  ARTIST WALL', 'top')}
      <div class="caption">
        <div class="label">02 담벼락</div>
        <h1>좋아하는 아티스트에게<br>한마디 남기기</h1>
        ${meta(VERSION, '02 / 02', 'HIPHOUSE.KR')}
      </div>`,
  },
]

const browser = await chromium.launch({ executablePath: exe })
for (const p of posters) {
  const page = await browser.newPage({ viewport: { width: 1080, height: 1350 }, deviceScaleFactor: 2 })
  const html = tpl.replace('{{cls}}', p.cls).replace('{{style}}', `--title:${p.title}px`).replace('{{body}}', p.body)
  await page.goto(`file://${process.cwd()}/update.html`, { waitUntil: 'load' }) // about:blank에서는 file:// 이미지 로드가 막혀 file 오리진 먼저 확보
  await page.setContent(html.replace('<head>', `<head><base href="file://${process.cwd()}/">`), { waitUntil: 'networkidle' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `${OUT}${p.name}.png` })
  await page.close()
  console.log('ok', p.name)
}
await browser.close()
