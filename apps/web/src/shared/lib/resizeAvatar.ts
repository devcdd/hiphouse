const AVATAR_SIDE = 256
const QUALITY = 0.85

// 가운데를 정사각형으로 잘라 256×256으로 줄인다. 서버는 256KB를 넘으면 거절하므로
// 휴대폰 원본 사진은 이 과정을 거쳐야 올라간다. canvas로 다시 그리면서 EXIF(촬영 위치 등)도 사라진다.
export async function resizeAvatar(file: File): Promise<Blob> {
  // createImageBitmap은 EXIF 회전값을 반영해서 그려 세로 사진이 눕지 않는다.
  const bitmap = await createImageBitmap(file).catch(() => {
    throw new Error('이미지를 읽지 못했습니다')
  })
  try {
    const side = Math.min(bitmap.width, bitmap.height)
    const canvas = document.createElement('canvas')
    canvas.width = AVATAR_SIDE
    canvas.height = AVATAR_SIDE
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('이미지를 처리하지 못했습니다')
    ctx.imageSmoothingQuality = 'high'
    ctx.drawImage(
      bitmap,
      (bitmap.width - side) / 2,
      (bitmap.height - side) / 2,
      side,
      side,
      0,
      0,
      AVATAR_SIDE,
      AVATAR_SIDE,
    )
    // WebP 인코딩을 못 하는 브라우저는 요청을 무시하고 PNG를 돌려주므로 그때는 JPEG로 다시 만든다.
    const webp = await toBlob(canvas, 'image/webp')
    if (webp?.type === 'image/webp') return webp
    const jpeg = await toBlob(canvas, 'image/jpeg')
    if (!jpeg) throw new Error('이미지를 처리하지 못했습니다')
    return jpeg
  } finally {
    bitmap.close()
  }
}

function toBlob(canvas: HTMLCanvasElement, type: string): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, QUALITY))
}
