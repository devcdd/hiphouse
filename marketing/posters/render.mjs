import { chromium } from 'playwright-core'
import { readFileSync } from 'node:fs'
const exe = process.env.HOME + '/Library/Caches/ms-playwright/chromium-1228/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing'
const tpl = readFileSync('poster.html', 'utf8')
const S = './shots/'
const top = `<div class="top"><img class="logo" src="logo.png" alt="힙집"><div class="pill">hiphouse.kr</div></div>`
const posters = [
  { name: 'hiphouse-poster-1', cls: 'cover', body: `${top}
    <div class="kicker">한국 힙합 앨범 아카이브</div>
    <h1>힙집</h1>
    <div class="sub">년도별 힙합 앨범과 수상작을 한곳에서 만나보세요.<br>평가하고, 즐겨찾기하고, 취향을 나눠보세요.</div>
    <div class="stage" style="top:580px"><div class="phone"><img src="${S}dark-home.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-2', body: `${top}
    <div class="kicker">01 · 연도 필터</div>
    <h1>그 해 힙합,<br><em>뭐 나왔더라?</em></h1>
    <div class="sub">새 앨범은 나오는 대로 바로바로 업데이트해요.<br>연도 하나 눌러서 그 해 앨범을 만나보세요.</div>
    <div class="tags"><span class="tag">All</span><span class="tag on">2023</span><span class="tag">2022</span><span class="tag">2021</span><span class="tag">2020</span></div>
    <div class="stage" style="top:640px"><div class="phone"><img src="${S}dark-home-2023.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-3', body: `${top}
    <div class="kicker">02 · 수상작</div>
    <h1>올해의 힙합 앨범,<br><em>한 번에 모아서</em></h1>
    <div class="sub">한국 힙합 어워즈·한국대중음악상 수상작만<br>모아서 들어보세요.</div>
    <div class="tags"><span class="tag">전체</span><span class="tag on">정규</span><span class="tag on">EP</span><span class="tag">싱글</span><span class="tag on">수상작</span></div>
    <div class="stage" style="top:660px"><div class="phone"><img src="${S}dark-home-2025-awarded.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-4', body: `${top}
    <div class="kicker">03 · 앨범 상세 · 미리듣기</div>
    <h1>레이블·장르·트랙까지<br><em>한 화면에</em></h1>
    <div class="sub">발매일·트랙 수·소개글·수상 경력을 확인하고<br>Spotify 플레이어로 바로 미리 들어보세요.</div>
    <div class="stage pair" style="top:560px"><div class="phone"><img src="${S}dark-album-kflip.png"></div><div class="phone"><img src="${S}dark-album-kflip-player.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-5', body: `${top}
    <div class="kicker">04 · 평가 · 댓글</div>
    <h1>별점 남기고<br><em>취향 나누기</em></h1>
    <div class="sub">5점 만점 별점과 댓글로 취향을 나눠보세요.<br>평가한 앨범은 마이페이지에서 다시 만나보세요.</div>
    <div class="stage" style="top:580px"><div class="phone" style="background:#0d0d0d"><img src="${S}dark-album-rating.png" style="transform:translateY(-28.5%)"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-poster-6', body: `${top}
    <div class="kicker">05 · 아티스트 · 검색</div>
    <h1>팔로우하고<br><em>신보 놓치지 않기</em></h1>
    <div class="sub">신보가 나오면 바로 업데이트되니 팔로우해보세요.<br>앨범·아티스트·트랙도 바로 검색해보세요.</div>
    <div class="stage pair" style="top:580px"><div class="phone"><img src="${S}dark-artist-beenzino.png"></div><div class="phone"><img src="${S}dark-search.png"></div></div><div class="fade"></div>` },
  { name: 'hiphouse-thumb', cls: 'thumb', body: `
    <div class="grid">${Array.from({ length: 20 }, (_, i) => `<img src="${S}covers/${i % 16}.jpg">`).join('')}</div>
    <div class="vignette"></div>
    <div class="kicker" style="margin-top:0">한국 힙합 앨범 아카이브</div>
    <h1>힙집</h1>
    <h2>년도별 힙합 앨범과 수상작을 한곳에서 만나보세요</h2>
    <div class="sub">평가하고, 즐겨찾기하고, 취향을 나눠보세요.</div>
    <div class="url">hiphouse.kr</div>` },
]
const browser = await chromium.launch({ executablePath: exe })
for (const p of posters) {
    const page = await browser.newPage({ viewport: { width: 1080, height: 1350 }, deviceScaleFactor: 2 })
  const html = tpl.replace('{{cls}}', p.cls ?? '').replace('{{bodycls}}', p.bodycls ?? '').replace('{{body}}', p.body)
  await page.goto(`file://${process.cwd()}/poster.html`, { waitUntil: "load" }) // about:blank에서는 file:// 이미지 로드가 막혀 file 오리진 먼저 확보
  await page.setContent(html.replace('<head>', `<head><base href="file://${process.cwd()}/">`), { waitUntil: 'networkidle' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(500)
  await page.screenshot({ path: `1.0.0/${p.name}.png` })
  await page.close()
  console.log('ok', p.name)
}
await browser.close()
