// 수축 필름으로 밀봉한 LP처럼 보이도록 커버 위에 비닐 층을 그린다.
// 선을 직접 긋지 않고 필름의 굴곡을 높이맵으로 만든 뒤 기울기로 반사를 계산한다. 그래야 넓은 반사 띠가
// 주름을 지나며 꺾이고, 주름 한쪽 면은 밝고 반대쪽은 어둡게 나와 긁힌 자국이 아닌 접힌 필름으로 읽힌다.

type Pt = [number, number]

interface Crease {
  pts: Pt[]
  // 능선에서 접힌 자국이 사라지는 곳까지의 거리(px).
  width: number
  // 능선 바로 옆의 기울기. 클수록 접힌 자국이 깊다. 음수면 골이 된다.
  slope: number
  glint: number
  // 반사선이 끊겨 보이는 간격(px)과 그 위상. 한 줄로 고르게 빛나면 유리에 간 금처럼 보인다.
  dash: number
  phase: number
  taper: (t: number) => number
  // 주름 양옆으로 넓고 완만하게 들뜬 면. 반사 띠가 주름 쪽으로 끌려와 휘는 건 이 면 때문이다.
  swell?: { width: number; slope: number }
}

// 파동 하나의 기울기. cos(x·kx + y·ky + 위상)을 픽셀마다 부르면 느려서 x와 y 쪽 값을 표로 나눠 둔다.
interface Wave {
  cosX: Float32Array
  sinX: Float32Array
  cosY: Float32Array
  sinY: Float32Array
  dx: number
  dy: number
}

interface Band {
  at: number
  half: number
  soft: number
  peak: number
  // 띠를 따라가는 방향으로 밝기가 첫 값에서 둘째 값으로 가며 차오른다. 끝없이 고른 띠는 유리판처럼 보인다.
  fade?: [from: number, to: number]
}

// 반사 띠에 수직인 방향. 띠는 왼쪽 아래에서 오른쪽 위로 가파르게 선다.
const BAND_NX = 0.899
const BAND_NY = 0.438

// at은 왼쪽 위 모서리에서 잰 거리를 커버 한 변으로 나눈 값이다. 0이 왼쪽 위, 1.337이 오른쪽 아래 모서리다.
// 커버 한가운데는 비워 두고 왼쪽 위와 오른쪽 아래에만 걸치게 해서 아트의 주제를 가리지 않는다.
// 범위 밖의 두 띠는 평평한 면에선 안 보이고, 모서리 주름이 그쪽으로 기울 때만 받아 비친다.
const BANDS: Band[] = [
  { at: -0.11, half: 0.08, soft: 0.02, peak: 0.36 },
  { at: 0.165, half: 0.016, soft: 0.006, peak: 0.2, fade: [-0.6, 0.04] },
  { at: 0.335, half: 0.09, soft: 0.007, peak: 0.18, fade: [-0.66, 0.04] },
  { at: 0.335, half: 0.1, soft: 0.08, peak: 0.06, fade: [-0.66, 0.04] },
  { at: 1.045, half: 0.05, soft: 0.007, peak: 0.12, fade: [0.26, -0.42] },
  { at: 1.47, half: 0.1, soft: 0.02, peak: 0.32 },
]

// 반사 띠가 기울기 1당 옮겨 가는 거리(px). 날카로운 능선까지 이만큼 옮기면 한 픽셀 사이에 띠를
// 건너뛰어 자글자글한 점이 생기므로, 능선은 따로 작은 값으로 꺾는다.
const BEND = 1500
const BEND_SHARP = 110
// 능선 꼭대기에 맺히는 가는 반사선과 그 둘레 번짐의 반폭(px).
const GLINT_W = 2.6
const BLOOM_W = 9

const LIGHT: [number, number, number] = [243, 248, 255]
const DARK: [number, number, number] = [8, 12, 22]

function mulberry(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

/** 같은 앨범은 다시 그려도 같은 주름이 나오게 id로 시드를 만든다. */
export function seedOf(text: string): number {
  let h = 2166136261
  for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), 16777619)
  return h >>> 0
}

function curve(x: number, y: number, angle: number, length: number, bend: number): Pt[] {
  const ex = x + Math.cos(angle) * length
  const ey = y + Math.sin(angle) * length
  const mx = (x + ex) / 2 - Math.sin(angle) * length * bend
  const my = (y + ey) / 2 + Math.cos(angle) * length * bend
  const pts: Pt[] = []
  for (let i = 0; i <= 8; i++) {
    const t = i / 8
    const a = (1 - t) * (1 - t)
    const b = 2 * (1 - t) * t
    const c = t * t
    pts.push([a * x + b * mx + c * ex, a * y + b * my + c * ey])
  }
  return pts
}

// 모서리나 가장자리에서 시작해 안쪽으로 갈수록 풀리는 주름.
const fromEdge = (t: number) => Math.min(1, t / 0.06) * (1 - t) ** 1.4
// 양 끝이 모두 풀리는, 면 위에 떠 있는 주름.
const floating = (t: number) => Math.sin(Math.PI * t) ** 0.8

// 커버 밖 (ox, oy)에서 (dx, dy)로 나아가는 직선이 커버로 들어오는 지점. 빗나가면 null.
function enterAt(ox: number, oy: number, dx: number, dy: number, n: number): Pt | null {
  let t0 = 0
  let t1 = Infinity
  for (const [o, d] of [
    [ox, dx],
    [oy, dy],
  ]) {
    if (Math.abs(d) < 1e-6) {
      if (o < 0 || o > n) return null
      continue
    }
    const a = -o / d
    const b = (n - o) / d
    t0 = Math.max(t0, Math.min(a, b))
    t1 = Math.min(t1, Math.max(a, b))
  }
  return t0 < t1 ? [ox + dx * t0, oy + dy * t0] : null
}

function buildCreases(n: number, rand: () => number): Crease[] {
  const out: Crease[] = []
  // 치수는 908px 커버 기준으로 잡았다.
  const u = n / 908
  const sign = () => (rand() < 0.8 ? 1 : -1)
  const between = (lo: number, hi: number) => lo + rand() * (hi - lo)
  const shimmer = () => ({ dash: between(34, 90) * u, phase: rand() * Math.PI * 2 })

  // 필름이 수축하며 모서리로 당겨져서 모서리마다 주름이 부채꼴로 퍼진다.
  const corners: [x: number, y: number, diag: number][] = [
    [0, 0, Math.PI * 0.25],
    [n, 0, Math.PI * 0.75],
    [n, n, Math.PI * 1.25],
    [0, n, Math.PI * 1.75],
  ]
  for (const [cx, cy, diag] of corners) {
    const strength = between(0.65, 1)

    // 모서리에 바짝 구겨진 짧은 주름.
    const tight = 4 + Math.floor(rand() * 3)
    for (let j = 0; j < tight; j++) {
      const inset = rand() * 5 * u
      out.push({
        pts: curve(
          cx + Math.cos(diag) * inset,
          cy + Math.sin(diag) * inset,
          diag + between(-0.7, 0.7),
          between(26, 80) * strength * u,
          between(-0.3, 0.3),
        ),
        width: between(5, 9) * u,
        slope: between(0.36, 0.66) * sign(),
        glint: between(0.7, 1),
        ...shimmer(),
        taper: fromEdge,
        swell: { width: between(24, 36) * u, slope: between(0.03, 0.07) },
      })
    }

    // 면 안쪽으로 길게 뻗는 주름. 모서리 한 점에서 뻗으면 금 간 유리처럼 보여서, 모서리 바깥의 한 점에서
    // 퍼지는 것으로 잡아 시작점이 양쪽 가장자리로 흩어지고 서로 나란하게 한다.
    const reach = between(70, 150) * u
    const ox = cx - Math.cos(diag) * reach
    const oy = cy - Math.sin(diag) * reach
    const long = 4 + Math.floor(rand() * 3)
    for (let j = 0; j < long; j++) {
      const off = ((j + 0.5) / long - 0.5) * 1.1 + between(-0.08, 0.08)
      const start = enterAt(ox, oy, Math.cos(diag + off), Math.sin(diag + off), n)
      if (!start) continue
      out.push({
        pts: curve(
          start[0],
          start[1],
          diag + off,
          between(110, 330) * (1 - Math.abs(off) * 0.7) * strength * u,
          between(-0.2, 0.2),
        ),
        width: between(8, 16) * u,
        slope: between(0.18, 0.36) * sign(),
        glint: between(0.55, 0.9),
        ...shimmer(),
        taper: fromEdge,
        swell: { width: between(45, 95) * u, slope: between(0.035, 0.085) },
      })
    }
  }

  // 가장자리를 따라 필름이 잘게 운 자국.
  const edges: [x: number, y: number, dx: number, dy: number, inward: number][] = [
    [0, 0, 1, 0, Math.PI * 0.5],
    [n, 0, 0, 1, Math.PI],
    [0, n, 1, 0, Math.PI * 1.5],
    [0, 0, 0, 1, 0],
  ]
  for (const [ex, ey, dx, dy, inward] of edges) {
    const count = 12 + Math.floor(rand() * 7)
    for (let j = 0; j < count; j++) {
      // 모서리에 가까울수록 촘촘하다.
      const r = rand() * 2 - 1
      const p = 0.5 + 0.5 * Math.sign(r) * Math.abs(r) ** 0.6
      const at = (0.04 + p * 0.92) * n
      out.push({
        pts: curve(ex + dx * at, ey + dy * at, inward + between(-0.6, 0.6), between(12, 50) * u, between(-0.2, 0.2)),
        width: between(4, 8) * u,
        slope: between(0.16, 0.42) * sign(),
        glint: between(0.4, 0.8),
        ...shimmer(),
        taper: fromEdge,
        swell: { width: between(24, 40) * u, slope: between(0.015, 0.04) },
      })
    }
  }

  for (let j = 0; j < 3; j++) {
    const side = Math.floor(rand() * 4)
    const along = between(0.2, 0.8) * n
    const depth = between(40, 130) * u
    const [x, y] = [
      [along, depth],
      [n - depth, along],
      [along, n - depth],
      [depth, along],
    ][side]
    out.push({
      pts: curve(x, y, rand() * Math.PI * 2, between(60, 170) * u, between(-0.25, 0.25)),
      width: between(8, 14) * u,
      slope: between(0.1, 0.26) * sign(),
      glint: 0.45,
      ...shimmer(),
      taper: floating,
      swell: { width: between(40, 70) * u, slope: between(0.02, 0.05) },
    })
  }
  return out
}

interface Field {
  n: number
  // 날카로운 접힌 자국. 명암과 가는 반사선을 만든다.
  sharp: Float32Array
  // 주름 둘레의 완만하게 들뜬 면. 반사 띠를 휘게 한다.
  swell: Float32Array
  glint: Float32Array
}

function addCrease(f: Field, c: Crease) {
  const { n } = f
  const lens: number[] = []
  let total = 0
  for (let i = 0; i < c.pts.length - 1; i++) {
    const l = Math.hypot(c.pts[i + 1][0] - c.pts[i][0], c.pts[i + 1][1] - c.pts[i][1])
    lens.push(l)
    total += l
  }
  if (total === 0) return
  const reach = Math.max(c.width, c.swell?.width ?? 0, BLOOM_W)
  const xs = c.pts.map((p) => p[0])
  const ys = c.pts.map((p) => p[1])
  const x0 = Math.max(0, Math.floor(Math.min(...xs) - reach))
  const x1 = Math.min(n - 1, Math.ceil(Math.max(...xs) + reach))
  const y0 = Math.max(0, Math.floor(Math.min(...ys) - reach))
  const y1 = Math.min(n - 1, Math.ceil(Math.max(...ys) + reach))
  if (x1 < x0 || y1 < y0) return
  const w = x1 - x0 + 1

  // 픽셀마다 가장 가까운 선분까지의 거리와 주름 위 위치를 모은다. 주름 전체를 감싼 사각형을 선분 수만큼
  // 훑으면 비스듬히 긴 주름에서 빈 칸을 도는 시간이 대부분이라, 선분마다 제 둘레만 훑는다.
  const far = Math.fround(reach * reach)
  const near = new Float32Array(w * (y1 - y0 + 1)).fill(far)
  const where = new Float32Array(near.length)
  let acc = 0
  for (let i = 0; i < lens.length; i++) {
    const [ax, ay] = c.pts[i]
    const [bx, by] = c.pts[i + 1]
    const vx = bx - ax
    const vy = by - ay
    const len2 = lens[i] * lens[i] || 1
    const sx0 = Math.max(x0, Math.floor(Math.min(ax, bx) - reach))
    const sx1 = Math.min(x1, Math.ceil(Math.max(ax, bx) + reach))
    const sy0 = Math.max(y0, Math.floor(Math.min(ay, by) - reach))
    const sy1 = Math.min(y1, Math.ceil(Math.max(ay, by) + reach))
    for (let y = sy0; y <= sy1; y++) {
      for (let x = sx0; x <= sx1; x++) {
        const s = Math.max(0, Math.min(1, ((x - ax) * vx + (y - ay) * vy) / len2))
        const d2 = (x - ax - vx * s) ** 2 + (y - ay - vy * s) ** 2
        const k = (y - y0) * w + (x - x0)
        if (d2 < near[k]) {
          near[k] = d2
          where[k] = (acc + s * lens[i]) / total
        }
      }
    }
    acc += lens[i]
  }

  const amp = (c.slope * c.width) / 2
  const swellAmp = c.swell ? (Math.sign(c.slope) * c.swell.slope * c.swell.width) / 2 : 0
  const shine = c.glint * Math.abs(c.slope)
  for (let k = 0; k < near.length; k++) {
    if (near[k] === far) continue
    const d = Math.sqrt(near[k])
    const t = c.taper(where[k])
    const i = (y0 + Math.floor(k / w)) * n + x0 + (k % w)
    if (d < c.width) f.sharp[i] += amp * t * (1 - d / c.width) ** 2
    if (c.swell && d < c.swell.width) f.swell[i] += swellAmp * t * (1 - d / c.swell.width) ** 2
    if (d < BLOOM_W) {
      const dash = 0.5 + 0.5 * Math.sin((where[k] * total * Math.PI * 2) / c.dash + c.phase)
      const lit = shine * t * (0.25 + 0.75 * dash ** 1.5)
      if (d < GLINT_W) f.glint[i] += lit * (1 - d / GLINT_W) ** 2
      f.glint[i] += lit * 0.22 * (1 - d / BLOOM_W) ** 2
    }
  }
}

const smoothstep = (a: number, b: number, v: number) => {
  const t = Math.max(0, Math.min(1, (v - a) / (b - a)))
  return t * t * (3 - 2 * t)
}

// 필름은 뽑아낸 방향으로 두께가 고르지 않아 반사가 가는 결로 갈라진다. 띠를 고르게 칠하면 유리판처럼 보인다.
function grain(q: number, along: number, phase: number[]): number {
  const p = q + 0.012 * Math.sin(along * 6.5 + phase[2])
  const v =
    0.55 * Math.sin(p * 150 + phase[3]) + 0.3 * Math.sin(p * 390 + phase[4]) + 0.15 * Math.sin(p * 870 + phase[5])
  return 0.76 + 0.34 * (0.5 + 0.5 * v) ** 1.4
}

function reflection(q: number, along: number, phase: number[]): number {
  let sum = 0
  for (const b of BANDS) {
    const edge = smoothstep(b.half + b.soft, b.half - b.soft, Math.abs(q - b.at))
    if (edge > 0) sum += b.peak * edge * (b.fade ? 0.22 + 0.78 * smoothstep(b.fade[0], b.fade[1], along) : 1)
  }
  return sum > 0 ? sum * grain(q, along, phase) : 0
}

/** (x, y)에서 한 변이 size인 정사각형 커버 위에 필름 층을 덧그린다. */
export function drawShrinkWrap(ctx: CanvasRenderingContext2D, x: number, y: number, size: number, seed: number) {
  const n = Math.round(size)
  const rand = mulberry(seed)
  const f: Field = { n, sharp: new Float32Array(n * n), swell: new Float32Array(n * n), glint: new Float32Array(n * n) }
  for (const c of buildCreases(n, rand)) addCrease(f, c)

  // 필름이 재킷 가장자리를 감싸며 꺾이는 부분. 위와 왼쪽 모서리는 빛을 받고 아래와 오른쪽은 그늘진다.
  // 고른 굵기면 테두리 선처럼 보여서 가장자리를 따라 꺾임 깊이를 출렁이게 한다.
  const roll = Math.max(3, Math.round(n * 0.0066))
  const phase = Array.from({ length: 6 }, () => rand() * Math.PI * 2)
  for (let py = 0; py < n; py++) {
    for (let px = 0; px < n; px++) {
      const e = Math.min(px, py, n - 1 - px, n - 1 - py)
      if (e >= roll) continue
      const s = ((px + py) / n) * Math.PI * 2
      const depth = 0.62 + 0.24 * Math.sin(s * 3.3 + phase[0]) + 0.14 * Math.sin(s * 9.1 + phase[1])
      f.sharp[py * n + px] -= roll * 0.3 * depth * (1 - e / roll) ** 2
    }
  }

  // 팽팽한 면도 완전히 평평하진 않아서 반사 띠가 느리게 출렁인다.
  // 긴 파동은 띠를 크게 휘고, 짧은 파동은 띠 가장자리를 잘게 흔든다.
  const wave = (length: number, slope: number): Wave => {
    const dir = rand() * Math.PI * 2
    const k = (Math.PI * 2) / (length * (n / 908))
    const [dx, dy] = [Math.cos(dir), Math.sin(dir)]
    const phase = rand() * Math.PI * 2
    return {
      cosX: Float32Array.from({ length: n }, (_, i) => Math.cos(i * dx * k)),
      sinX: Float32Array.from({ length: n }, (_, i) => Math.sin(i * dx * k)),
      cosY: Float32Array.from({ length: n }, (_, i) => Math.cos(i * dy * k + phase) * slope),
      sinY: Float32Array.from({ length: n }, (_, i) => Math.sin(i * dy * k + phase) * slope),
      dx,
      dy,
    }
  }
  const waves: Wave[] = [
    ...Array.from({ length: 4 }, () => wave(260 + rand() * 380, 0.006 + rand() * 0.01)),
    ...Array.from({ length: 4 }, () => wave(70 + rand() * 90, 0.0012 + rand() * 0.0018)),
  ]

  const img = new ImageData(n, n)
  const out = img.data
  for (let py = 0; py < n; py++) {
    for (let px = 0; px < n; px++) {
      const i = py * n + px
      const l = i - (px > 0 ? 1 : 0)
      const r = i + (px < n - 1 ? 1 : 0)
      const t = i - (py > 0 ? n : 0)
      const b = i + (py < n - 1 ? n : 0)
      const hx = (f.sharp[r] - f.sharp[l]) * 0.5
      const hy = (f.sharp[b] - f.sharp[t]) * 0.5
      let sx = (f.swell[r] - f.swell[l]) * 0.5
      let sy = (f.swell[b] - f.swell[t]) * 0.5
      for (const w of waves) {
        const c = w.cosX[px] * w.cosY[py] - w.sinX[px] * w.sinY[py]
        sx += c * w.dx
        sy += c * w.dy
      }

      const ux = px - BEND * sx - BEND_SHARP * hx
      const uy = py - BEND * sy - BEND_SHARP * hy
      const glare = reflection((ux * BAND_NX + uy * BAND_NY) / n, (ux * BAND_NY - uy * BAND_NX) / n, phase)

      // 왼쪽 위에서 비추는 빛. 그쪽을 향한 면은 밝고 등진 면은 어둡다.
      const lit = (hx + sx * 0.6) * 0.55 + (hy + sy * 0.6) * 0.83
      const fall = (px + py) / (2 * n)
      // 능선의 반사선은 광원이 비치는 띠 안에서 가장 세게 빛난다.
      const spark = Math.min(0.9, f.glint[i] * (1.5 + glare * 6))

      const white = Math.min(0.94, glare + Math.min(0.5, Math.max(0, lit) * 1.2) + spark + 0.03 * (1 - fall) ** 2)
      // 흰 반사는 밝은 커버에서 묻힌다. 필름 전체를 아주 조금 눌러 두면 반사가 그만큼 도드라진다.
      const black = Math.min(0.6, 0.035 + Math.min(0.45, Math.max(0, -lit)) + 0.05 * fall * fall)
      const alpha = 1 - (1 - black) * (1 - white)
      const mix = alpha > 0 ? white / alpha : 0
      const o = i * 4
      out[o] = DARK[0] + (LIGHT[0] - DARK[0]) * mix
      out[o + 1] = DARK[1] + (LIGHT[1] - DARK[1]) * mix
      out[o + 2] = DARK[2] + (LIGHT[2] - DARK[2]) * mix
      // 넓게 번지는 옅은 반사는 8비트에서 계단이 지므로 알파를 살짝 흩뜨린다.
      out[o + 3] = Math.max(0, Math.min(255, alpha * 255 + (rand() - 0.5) * 1.6))
    }
  }

  const layer = document.createElement('canvas')
  layer.width = n
  layer.height = n
  layer.getContext('2d')?.putImageData(img, 0, 0)
  ctx.drawImage(layer, x, y, size, size)
}
