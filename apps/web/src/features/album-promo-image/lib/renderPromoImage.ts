import type { Album, Track } from '@/entities/album'
import { displayName } from '@/shared/lib/displayName'
import { albumStats } from './albumStats'
import { drawShrinkWrap, seedOf } from './shrinkWrap'

// 인스타그램 피드 세로형(4:5).
const W = 1080
const H = 1350
// 커버와 트랙리스트 카드가 같은 여백을 쓴다. 양옆은 PAD, 위에는 워드마크 띠(TOP), 맨 아래 줄은 BOTTOM_Y에 놓는다.
const PAD = 86
const TOP = 118
const BOTTOM_Y = H - 64
const COVER = W - PAD * 2

const UI = 'Pretendard, -apple-system, BlinkMacSystemFont, "Apple SD Gothic Neo", sans-serif'
// 라틴 문자와 숫자는 Anton, 한글은 Black Han Sans가 맡는다.
const DISPLAY = `Anton, "Black Han Sans", ${UI}`
const MONO = '"JetBrains Mono", ui-monospace, Menlo, monospace'

const ROW_H = 56
// 크레딧을 곡명 아래 둘째 줄에 쓰는 곡의 줄 높이.
const ROW_H_CREDIT = 72
const LIST_TOP = 422
const LIST_H = BOTTOM_Y - 40 - LIST_TOP

const CDN = 'https://cdn.jsdelivr.net/npm/'
const FONTS: [family: string, url: string, descriptors?: FontFaceDescriptors][] = [
  ['Pretendard', 'pretendard@1.3.9/dist/web/static/woff2/Pretendard-Medium.woff2', { weight: '500' }],
  ['Pretendard', 'pretendard@1.3.9/dist/web/static/woff2/Pretendard-Bold.woff2', { weight: '700' }],
  ['Anton', '@fontsource/anton@5.2.5/files/anton-latin-400-normal.woff2'],
  [
    'Black Han Sans',
    '@fontsource/black-han-sans@5.2.5/files/black-han-sans-korean-400-normal.woff2',
    { unicodeRange: 'U+1100-11FF, U+3130-318F, U+AC00-D7AF' },
  ],
  ['JetBrains Mono', '@fontsource/jetbrains-mono@5.2.5/files/jetbrains-mono-latin-500-normal.woff2', { weight: '500' }],
]
let fontsReady: Promise<void> | null = null

// 사이트 본문은 시스템 폰트라 캔버스에서 쓸 서체를 따로 받는다.
function loadFonts(): Promise<void> {
  fontsReady ??= Promise.all(
    FONTS.map(([family, url, desc]) =>
      new FontFace(family, `url(${CDN}${url})`, desc).load().then((f) => void document.fonts.add(f)),
    ),
  )
    .then(() => undefined)
    // 폰트를 못 받아도 대체 서체로 그린다. 다음 클릭 때 다시 시도한다.
    .catch(() => {
      fontsReady = null
    })
  return fontsReady
}

// Apple 아트워크 URL은 끝의 크기 지정만 바꾸면 고해상도를 준다. Spotify는 640px이 최대다.
function hiResArt(url: string): string {
  return url.replace(/\/\d+x\d+bb\.(jpg|png|webp)$/, '/1200x1200bb.jpg')
}

// <img>가 CORS 없이 캐시한 응답을 재사용하면 캔버스가 오염돼 내보내기가 막혀서 fetch로 따로 받는다.
async function loadArt(url: string): Promise<ImageBitmap> {
  const res = await fetch(hiResArt(url), { mode: 'cors' })
  if (!res.ok) throw new Error(`앨범아트를 받지 못했습니다 (${res.status})`)
  return createImageBitmap(await res.blob())
}

export type RGB = [number, number, number]

// 커버에서 뽑은 색이 탁하거나 시리즈 톤을 맞추고 싶을 때 고르는 배경색. 트랙리스트 장은 여기서 톤만 옮긴다.
export const PROMO_BACKGROUNDS: { id: string; name: string; rgb: RGB }[] = [
  { id: 'ink', name: '먹색', rgb: [20, 20, 20] },
  { id: 'paper', name: '종이', rgb: [236, 232, 223] },
  { id: 'concrete', name: '콘크리트', rgb: [150, 150, 146] },
  { id: 'burgundy', name: '버건디', rgb: [94, 22, 34] },
  { id: 'red', name: '레드', rgb: [178, 44, 34] },
  { id: 'orange', name: '오렌지', rgb: [232, 106, 28] },
  { id: 'mustard', name: '머스터드', rgb: [214, 170, 52] },
  { id: 'olive', name: '올리브', rgb: [92, 98, 52] },
  { id: 'forest', name: '포레스트', rgb: [24, 64, 48] },
  { id: 'navy', name: '네이비', rgb: [22, 34, 72] },
]

interface Palette {
  bg: string
  fg: string
  sub: string
  rule: string
}

// 배경 밝기에 맞춰 글자색(먹색이나 종이색)과 보조색, 구분선 색을 정한다.
function paletteOf(rgb: RGB): Palette {
  const lum = (0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2]) / 255
  const ink = lum < 0.5 ? '244,241,234' : '17,17,17'
  return {
    bg: `rgb(${rgb.join(',')})`,
    fg: `rgb(${ink})`,
    sub: `rgba(${ink},0.64)`,
    rule: `rgba(${ink},0.16)`,
  }
}

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  r /= 255
  g /= 255
  b /= 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const l = (max + min) / 2
  if (max === min) return [0, 0, l]
  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  const h = max === r ? (g - b) / d + (g < b ? 6 : 0) : max === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [h * 60, s, l]
}

function hslToRgb(h: number, s: number, l: number): RGB {
  const k = (n: number) => (n + h / 30) % 12
  const a = s * Math.min(l, 1 - l)
  const f = (n: number) => l - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1))
  return [f(0), f(8), f(4)].map((v) => Math.round(v * 255)) as RGB
}

// 트랙리스트 배경은 커버 카드 색에서 채도를 15% 빼고 밝기를 반대쪽으로 한 단계 옮겨, 같은 톤이되 구분되게 한다.
function shiftTone(rgb: RGB): RGB {
  const [h, s, l] = rgbToHsl(...rgb)
  return hslToRgb(h, s * 0.85, l < 0.5 ? Math.min(l + 0.08, 0.95) : Math.max(l - 0.08, 0.05))
}

// 커버에서 가장 눈에 띄는 색 하나를 뽑아 배경으로 쓴다. 색상환을 24칸으로 나눠 채도 높은 픽셀이
// 가장 많이 모인 칸의 평균색을 고르고, 흑백에 가까운 커버면 밝기에 따라 먹색이나 종이색을 쓴다.
function pickColor(art: ImageBitmap): RGB {
  const n = 48
  const c = document.createElement('canvas')
  c.width = n
  c.height = n
  const cx = c.getContext('2d', { willReadFrequently: true })
  if (!cx) return [20, 20, 20]
  cx.drawImage(art, 0, 0, n, n)
  const px = cx.getImageData(0, 0, n, n).data
  const buckets = Array.from({ length: 24 }, () => ({ w: 0, r: 0, g: 0, b: 0 }))
  let lightSum = 0
  for (let i = 0; i < px.length; i += 4) {
    const [r, g, b] = [px[i], px[i + 1], px[i + 2]]
    const [h, s, l] = rgbToHsl(r, g, b)
    lightSum += l
    if (s < 0.3 || l < 0.12 || l > 0.85) continue
    const w = s * (1 - Math.abs(l - 0.5))
    const k = buckets[Math.floor(h / 15) % 24]
    k.w += w
    k.r += r * w
    k.g += g * w
    k.b += b * w
  }
  const best = buckets.reduce((a, b) => (b.w > a.w ? b : a))
  if (best.w > n * n * 0.04) return [best.r / best.w, best.g / best.w, best.b / best.w].map(Math.round) as RGB
  return lightSum / (n * n) > 0.5 ? [236, 232, 223] : [20, 20, 20]
}

// 글자가 maxWidth를 넘으면 sizes 순서대로 줄여 보고, 가장 작은 크기로도 넘치면 말줄임한다.
function fitText(ctx: CanvasRenderingContext2D, text: string, font: (size: number) => string, sizes: number[], maxWidth: number) {
  for (const size of sizes) {
    ctx.font = font(size)
    if (ctx.measureText(text).width <= maxWidth) return text
  }
  let t = text
  while (t.length > 1 && ctx.measureText(`${t}…`).width > maxWidth) t = t.slice(0, -1)
  return `${t.trimEnd()}…`
}

const ui = (weight: number) => (size: number) => `${weight} ${size}px ${UI}`
const display = (size: number) => `400 ${size}px ${DISPLAY}`

function monoLabel(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, size = 18) {
  ctx.font = `500 ${size}px ${MONO}`
  const prev = ctx.letterSpacing
  ctx.letterSpacing = '0.12em'
  ctx.fillText(text, x, y)
  ctx.letterSpacing = prev
}

function wrap(ctx: CanvasRenderingContext2D, text: string, maxW: number): string[] {
  const lines: string[] = []
  for (const word of text.split(' ')) {
    const last = lines[lines.length - 1]
    if (last != null && ctx.measureText(`${last} ${word}`).width <= maxW) lines[lines.length - 1] = `${last} ${word}`
    else lines.push(word)
  }
  return lines
}

// 제목을 한 줄로 최대한 크게 쓰고, 넘치면 크기를 줄여 가며 두 줄로 접는다. 커버 아래 띠가 좁아서 세 줄은 쓰지 않는다.
// 끝까지 넘치면 (띄어쓰기 없는 긴 단어) 카드 밖으로 잘리게 둔다.
function layoutTitle(ctx: CanvasRenderingContext2D, text: string, maxW: number) {
  const steps: [size: number, maxLines: number][] = [
    [136, 1], [120, 1], [106, 1], [94, 1], [84, 1], [74, 1],
    [66, 2], [58, 2], [52, 2],
  ]
  for (const [size, maxLines] of steps) {
    ctx.font = display(size)
    const lines = wrap(ctx, text, maxW)
    if (lines.length <= maxLines && lines.every((l) => ctx.measureText(l).width <= maxW)) return { size, lines }
  }
  ctx.font = display(52)
  return { size: 52, lines: wrap(ctx, text, maxW).slice(0, 2) }
}

function drawLogo(ctx: CanvasRenderingContext2D, color: string) {
  const size = 32
  ctx.save()
  ctx.fillStyle = color
  ctx.font = display(size)
  ctx.letterSpacing = '0.12em'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  // 자간이 마지막 글자 뒤에도 붙어 오른쪽으로 밀리니, 그 절반만큼 오른쪽에서 그려 가운데를 맞춘다.
  ctx.fillText('HIPHOUSE', W / 2 + size * 0.06, TOP / 2 + 2)
  ctx.restore()
}

// 검정 스티커에 샴페인 골드 박을 찍은 느낌을 내려고 밝은 띠와 어두운 띠를 비스듬히 번갈아 둔다.
const FOIL_STOPS: [at: number, color: string][] = [
  [0, '#dcc289'],
  [0.24, '#b9945b'],
  [0.46, '#f7ecce'],
  [0.56, '#e2ca95'],
  [0.8, '#b18b52'],
  [1, '#dfc790'],
]

// 필름 위에 손으로 붙인 신보 스티커. 커버 색에 따라 달라지면 게시물마다 다른 물건처럼 보여서
// 검정 바탕에 금박으로 고정해 힙집 표식으로 쓴다. 기울인 뒤에도 커버 오른쪽 아래 모서리에서 margin만큼 안쪽에 놓인다.
function drawSticker(ctx: CanvasRenderingContext2D, label: string, sub: string) {
  const size = 58
  const tracking = 0.05
  const tilt = -0.07
  const margin = 24
  ctx.save()
  ctx.font = display(size)
  ctx.letterSpacing = `${tracking}em`
  // 자간은 마지막 글자 뒤에도 붙으니 그만큼 빼야 글자가 스티커 가운데에 온다.
  const textW = ctx.measureText(label).width - size * tracking
  const capH = ctx.measureText('H').actualBoundingBoxAscent
  const w = textW + 76
  const h = 138
  const radius = 12
  // 기념일 문구처럼 긴 글자는 스티커가 커버 절반을 넘지 않게 통째로 줄인다.
  const scale = Math.min(1, (COVER * 0.47) / w)
  const [sin, cos] = [Math.abs(Math.sin(tilt)), Math.cos(tilt)]
  ctx.translate(
    PAD + COVER - margin - ((w * cos + h * sin) * scale) / 2,
    TOP + COVER - margin - ((w * sin + h * cos) * scale) / 2,
  )
  ctx.rotate(tilt)
  ctx.scale(scale, scale)

  const body = ctx.createLinearGradient(0, -h / 2, 0, h / 2)
  body.addColorStop(0, '#1d1d1e')
  body.addColorStop(1, '#0a0a0b')
  ctx.fillStyle = body
  // 스티커는 필름에 붙어 있어서 그림자가 짧다. 넓은 그림자는 옅게만 깔아 윤곽을 살린다.
  ctx.shadowColor = 'rgba(0,0,0,0.3)'
  ctx.shadowBlur = 18
  ctx.shadowOffsetY = 7
  ctx.beginPath()
  ctx.roundRect(-w / 2, -h / 2, w, h, radius)
  ctx.fill()
  ctx.shadowColor = 'rgba(0,0,0,0.5)'
  ctx.shadowBlur = 4
  ctx.shadowOffsetY = 1.5
  ctx.fill()
  ctx.shadowColor = 'transparent'

  // 코팅된 스티커 면에 비친 빛. 필름과 같은 방향으로 기울여 한 조명 아래 있는 것처럼 맞춘다.
  ctx.save()
  ctx.clip()
  const gloss = ctx.createLinearGradient(-w * 0.42, -h / 2, -w * 0.12, h / 2)
  gloss.addColorStop(0, 'rgba(255,255,255,0)')
  gloss.addColorStop(0.42, 'rgba(255,255,255,0.075)')
  gloss.addColorStop(0.5, 'rgba(255,255,255,0.075)')
  gloss.addColorStop(0.52, 'rgba(255,255,255,0)')
  ctx.fillStyle = gloss
  ctx.fillRect(-w / 2, -h / 2, w, h)
  ctx.restore()

  // 잘라 낸 단면이 빛을 받아 종이 두께가 보인다. 어두운 커버에서는 이 선이 스티커 윤곽이 된다.
  const rim = ctx.createLinearGradient(0, -h / 2, 0, h / 2)
  rim.addColorStop(0, 'rgba(255,255,255,0.36)')
  rim.addColorStop(0.3, 'rgba(255,255,255,0.12)')
  rim.addColorStop(1, 'rgba(255,255,255,0.06)')
  ctx.strokeStyle = rim
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.roundRect(-w / 2 + 0.5, -h / 2 + 0.5, w - 1, h - 1, radius - 0.5)
  ctx.stroke()

  const foil = ctx.createLinearGradient(-w / 2, -h, w / 2, h)
  for (const [at, color] of FOIL_STOPS) foil.addColorStop(at, color)
  // 검정 커버 위에서는 스티커 바탕이 묻히므로 안쪽 금박 테두리가 윤곽 노릇을 한다.
  ctx.strokeStyle = foil
  ctx.lineWidth = 1.5
  ctx.beginPath()
  ctx.roundRect(-w / 2 + 8, -h / 2 + 8, w - 16, h - 16, radius - 5)
  ctx.stroke()

  // 큰 글자와 날짜 줄을 한 덩어리로 보고 스티커 세로 가운데에 맞춘다.
  const gap = 24
  const subH = 10
  const baseline = -(capH + gap + subH) / 2 + capH
  ctx.fillStyle = foil
  ctx.textAlign = 'left'
  ctx.textBaseline = 'alphabetic'
  ctx.fillText(label, -textW / 2, baseline)

  ctx.font = `500 15px ${MONO}`
  ctx.letterSpacing = '0.2em'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const subY = baseline + gap + subH / 2
  const subW = ctx.measureText(sub).width
  ctx.fillText(sub, 15 * 0.1, subY)
  ctx.fillRect(-subW / 2 - 42, subY - 0.5, 28, 1)
  ctx.fillRect(subW / 2 + 14, subY - 0.5, 28, 1)
  ctx.restore()
}

const TYPE_EN: Record<string, string> = { 정규: 'ALBUM', EP: 'EP', 싱글: 'SINGLE', 컴필레이션: 'COMPILATION' }

function metaLine(album: Album, tracks: Track[]): string {
  const { count, minutes } = albumStats(album, tracks)
  const date = album.release_date?.replaceAll('-', '.') ?? (album.year != null ? String(album.year) : '')
  const type = album.type_label ? (TYPE_EN[album.type_label] ?? album.type_label) : ''
  return [date, type, count > 0 ? `${count} ${count === 1 ? 'TRACK' : 'TRACKS'}` : '', minutes != null ? `${minutes} MIN` : '']
    .filter(Boolean)
    .join('  /  ')
}

function newCanvas(): [HTMLCanvasElement, CanvasRenderingContext2D] {
  const canvas = document.createElement('canvas')
  canvas.width = W
  canvas.height = H
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('캔버스를 만들 수 없습니다')
  return [canvas, ctx]
}

// 커버를 바닥에 놓인 밀봉 LP처럼 그린다. 그림자로 띄우고 아트 위에 수축 필름을 씌운다.
function drawJacket(ctx: CanvasRenderingContext2D, art: ImageBitmap, seed: number) {
  ctx.save()
  ctx.beginPath()
  ctx.roundRect(PAD, TOP, COVER, COVER, 3)
  ctx.fillStyle = '#000'
  ctx.shadowColor = 'rgba(0,0,0,0.3)'
  ctx.shadowBlur = 44
  ctx.shadowOffsetY = 22
  ctx.fill()
  // 넓은 그림자만 있으면 재킷이 떠 보여서 바닥에 닿는 짙은 그림자를 한 겹 더 둔다.
  ctx.shadowColor = 'rgba(0,0,0,0.32)'
  ctx.shadowBlur = 9
  ctx.shadowOffsetY = 3
  ctx.fill()
  ctx.shadowColor = 'transparent'
  ctx.clip()
  ctx.drawImage(art, PAD, TOP, COVER, COVER)
  drawShrinkWrap(ctx, PAD, TOP, COVER, seed)
  ctx.restore()
}

function ordinal(n: number): string {
  const tail = n % 100 >= 11 && n % 100 <= 13 ? 'TH' : (['TH', 'ST', 'ND', 'RD'][n % 10] ?? 'TH')
  return `${n}${tail}`
}

// 커버를 위쪽 가운데에 두고, 아래 띠에는 메타 줄부터 아티스트, 제목 순으로 밑에서부터 쌓아 글자를 하단에 붙인다.
function drawCover(
  ctx: CanvasRenderingContext2D,
  album: Album,
  tracks: Track[],
  art: ImageBitmap,
  pal: Palette,
  opts: PromoOptions,
) {
  ctx.fillStyle = pal.bg
  ctx.fillRect(0, 0, W, H)
  drawJacket(ctx, art, seedOf(album.id))
  const date = album.release_date?.replaceAll('-', '.')
  if (opts.anniversaryYears) {
    drawSticker(ctx, `${ordinal(opts.anniversaryYears)} ANNIVERSARY`, `RELEASED ${date ?? ''}`)
  } else {
    drawSticker(ctx, 'NEW RELEASE', date ?? 'OUT NOW')
  }

  drawLogo(ctx, pal.fg)

  const left = PAD
  const maxW = COVER
  const metaY = BOTTOM_Y

  ctx.textBaseline = 'middle'
  ctx.fillStyle = pal.sub
  monoLabel(ctx, metaLine(album, tracks), left, metaY)

  // 아티스트는 제목 위에 작은 라벨로 두고, 제목은 메타 줄 바로 위에서부터 위로 쌓는다.
  ctx.fillStyle = pal.fg
  ctx.textAlign = 'left'
  ctx.textBaseline = 'alphabetic'
  const title = layoutTitle(ctx, displayName(album), maxW)
  ctx.font = display(title.size)
  const lh = title.size * 1.02
  const lastBaseline = metaY - 66
  const firstBaseline = lastBaseline - lh * (title.lines.length - 1)
  title.lines.forEach((line, i) => ctx.fillText(line, left, firstBaseline + lh * i))
  const capTop = firstBaseline - ctx.measureText('H').actualBoundingBoxAscent

  ctx.fillStyle = pal.sub
  const artist = fitText(ctx, album.artists.map(displayName).join(', '), ui(700), [30, 28, 26], maxW)
  ctx.letterSpacing = '0.02em'
  ctx.fillText(artist, left, capTop - 20)
  ctx.letterSpacing = '0px'
}

function duration(ms: number | null): string {
  if (ms == null) return ''
  const s = Math.round(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

const creditNames = (track: Track) => track.artists.map((a) => a.name).filter(Boolean)

// Apple 트랙은 id 없이 "A, B & C"로 합친 이름 하나만 줘서 구분자로 인원을 센다.
function isJoint(track: Track): boolean {
  return creditNames(track).flatMap((n) => n.split(/\s*(?:,|\s&\s)\s*/)).filter(Boolean).length > 1
}

// 두 명 이상 참여한 곡이 하나라도 있으면 모든 곡에 크레딧 줄을 붙여 줄 모양을 맞추고, 없으면 곡명만 쓴다.
function trackCredits(tracks: Track[]): string[] {
  const show = tracks.some(isJoint)
  return tracks.map((t) => (show ? creditNames(t).join(', ') : ''))
}

interface Row {
  no: number
  track: Track
  credit: string
}

const rowHeight = (r: Row) => (r.credit ? ROW_H_CREDIT : ROW_H)

function drawTracklist(
  ctx: CanvasRenderingContext2D,
  album: Album,
  tracks: Track[],
  rows: Row[],
  page: string,
  pal: Palette,
) {
  const { fg, sub } = pal
  ctx.fillStyle = pal.bg
  ctx.fillRect(0, 0, W, H)
  drawLogo(ctx, fg)

  const maxW = W - PAD * 2
  ctx.fillStyle = fg
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'
  monoLabel(ctx, page ? `TRACKLIST  ${page}` : 'TRACKLIST', PAD, 152)

  // 커버 카드처럼 아티스트를 제목 위 작은 라벨로 둔다. 제목 기준선을 고정해 제목 크기가 달라도 메타 줄과의 간격이 같다.
  const titleBaseline = 316
  ctx.textBaseline = 'alphabetic'
  ctx.fillStyle = fg
  const title = fitText(ctx, displayName(album), display, [104, 92, 82, 72, 64], maxW)
  ctx.fillText(title, PAD, titleBaseline)
  const capTop = titleBaseline - ctx.measureText('H').actualBoundingBoxAscent
  ctx.fillStyle = sub
  ctx.letterSpacing = '0.02em'
  ctx.fillText(fitText(ctx, album.artists.map(displayName).join(', '), ui(700), [26, 24, 22], maxW), PAD, capTop - 20)
  ctx.letterSpacing = '0px'
  ctx.textBaseline = 'middle'
  ctx.fillStyle = sub
  monoLabel(ctx, metaLine(album, tracks), PAD, 364, 16)

  ctx.fillStyle = fg
  ctx.fillRect(PAD, LIST_TOP - 24, maxW, 2)

  const titleX = PAD + 64
  const titleMax = W - PAD - 90 - titleX
  let rowTop = LIST_TOP
  rows.forEach((row, i) => {
    if (i > 0) {
      ctx.fillStyle = pal.rule
      ctx.fillRect(PAD, rowTop, maxW, 1)
    }
    const y = row.credit ? rowTop + 24 : rowTop + ROW_H / 2
    // 숫자마다 폭이 달라(11이 10보다 좁음) 왼쪽 정렬이면 들쭉날쭉해 보여서 번호 열 오른쪽 끝에 맞춘다.
    ctx.textAlign = 'right'
    ctx.fillStyle = fg
    ctx.font = `500 20px ${MONO}`
    ctx.fillText(String(row.no).padStart(2, '0'), PAD + 30, y)
    ctx.fillStyle = sub
    ctx.font = `500 18px ${MONO}`
    ctx.fillText(duration(row.track.duration_ms), W - PAD, y)

    ctx.textAlign = 'left'
    ctx.fillStyle = fg
    ctx.fillText(fitText(ctx, displayName(row.track), ui(500), [25, 23, 21], titleMax), titleX, y)
    if (row.credit) {
      ctx.fillStyle = sub
      ctx.fillText(fitText(ctx, row.credit, ui(500), [18], titleMax), titleX, y + 28)
    }
    rowTop += rowHeight(row)
  })

  ctx.fillStyle = fg
  ctx.textAlign = 'right'
  // 자간이 마지막 글자 뒤에도 붙어서 그만큼 밀어 재생 시간 열과 오른쪽 끝을 맞춘다.
  monoLabel(ctx, 'HIPHOUSE.KR', W - PAD + 18 * 0.12, BOTTOM_Y)
}

// 크레딧 줄이 있는 곡은 높이가 달라 곡 수가 아닌 높이 합으로 나눈다. 넘치는 만큼 장을 늘리되,
// 마지막 장만 한두 곡 남지 않게 장마다 높이를 고르게 맞춘다.
function paginate(rows: Row[]): Row[][] {
  const split = (limit: number) => {
    const pages: Row[][] = [[]]
    let used = 0
    for (const r of rows) {
      const h = rowHeight(r)
      if (pages[pages.length - 1].length > 0 && used + h > limit) {
        pages.push([])
        used = 0
      }
      pages[pages.length - 1].push(r)
      used += h
    }
    return pages
  }
  const greedy = split(LIST_H)
  const total = rows.reduce((sum, r) => sum + rowHeight(r), 0)
  const balanced = split(Math.min(LIST_H, Math.ceil(total / greedy.length) + ROW_H - 1))
  return balanced.length === greedy.length ? balanced : greedy
}

function toBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('이미지를 만들지 못했습니다'))), 'image/png'),
  )
}

/** 첫 장은 커버, 둘째 장부터 트랙리스트. 트랙이 하나 이하면 커버 한 장만 만든다. */
export interface PromoOptions {
  // 발매 기념일 포스터면 몇 주년인지. 없으면 신보 포스터로 그린다.
  anniversaryYears?: number
  // 없으면 커버에서 가장 눈에 띄는 색을 뽑아 쓴다.
  background?: RGB
}

export async function renderPromoImages(album: Album, tracks: Track[], opts: PromoOptions = {}): Promise<Blob[]> {
  if (!album.image_url) throw new Error('앨범아트가 없습니다')
  const [art] = await Promise.all([loadArt(album.image_url), loadFonts()])
  try {
    const [cover, coverCtx] = newCanvas()
    const color = opts.background ?? pickColor(art)
    drawCover(coverCtx, album, tracks, art, paletteOf(color), opts)
    const listPal = paletteOf(shiftTone(color))
    const out = [toBlob(cover)]
    if (tracks.length > 1) {
      const credits = trackCredits(tracks)
      const pages = paginate(tracks.map((track, i) => ({ no: i + 1, track, credit: credits[i] })))
      pages.forEach((rows, i) => {
        const [canvas, ctx] = newCanvas()
        drawTracklist(ctx, album, tracks, rows, pages.length > 1 ? `${i + 1}/${pages.length}` : '', listPal)
        out.push(toBlob(canvas))
      })
    }
    return await Promise.all(out)
  } finally {
    art.close()
  }
}

export function promoFileName(album: Album, index: number, opts: PromoOptions = {}): string {
  const artist = album.artists[0] ? displayName(album.artists[0]) : ''
  const base = [artist, displayName(album)].filter(Boolean).join(' - ')
  const kind = opts.anniversaryYears ? ` ${opts.anniversaryYears}주년` : ''
  return `hiphouse${kind} ${base} ${index + 1}`.replace(/[\\/:*?"<>|]/g, '_') + '.png'
}
