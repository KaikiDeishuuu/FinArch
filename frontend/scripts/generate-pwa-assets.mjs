import { readFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright'

const frontendDir = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const publicDir = resolve(frontendDir, 'public')

const rasterTargets = [
  { source: 'pwa-icon.svg', output: 'pwa-192.png', width: 192, height: 192 },
  { source: 'pwa-icon.svg', output: 'pwa-512.png', width: 512, height: 512 },
  { source: 'pwa-maskable.svg', output: 'pwa-maskable-192.png', width: 192, height: 192 },
  { source: 'pwa-maskable.svg', output: 'pwa-maskable-512.png', width: 512, height: 512 },
  { source: 'pwa-icon.svg', output: 'apple-touch-icon.png', width: 180, height: 180 },
  { source: 'screenshot-narrow.svg', output: 'screenshot-narrow.png', width: 390, height: 844 },
]

const splashTargets = [
  { width: 1179, height: 2556, scale: 3 },
  { width: 1170, height: 2532, scale: 3 },
  { width: 1125, height: 2436, scale: 3 },
  { width: 1284, height: 2778, scale: 3 },
  { width: 828, height: 1792, scale: 2 },
]

const fontPackages = [
  {
    specifier: '@fontsource-variable/geist',
    sourceFamily: 'Geist Variable',
    targetFamily: 'Geist',
  },
  {
    specifier: '@fontsource-variable/noto-sans-sc',
    sourceFamily: 'Noto Sans SC Variable',
    targetFamily: 'Noto Sans SC',
  },
]

async function loadBundledFontCss() {
  const styles = await Promise.all(fontPackages.map(async ({ specifier, sourceFamily, targetFamily }) => {
    const cssPath = fileURLToPath(import.meta.resolve(specifier))
    const cssDir = dirname(cssPath)
    let css = await readFile(cssPath, 'utf8')
    const references = [...new Set(
      [...css.matchAll(/url\(([^)]+)\)/g)].map((match) => match[1].trim()),
    )]

    for (const reference of references) {
      const relativePath = reference.replace(/^(['"])(.*)\1$/, '$2')
      if (!relativePath.startsWith('./')) {
        throw new Error(`Unexpected font asset URL in ${specifier}: ${relativePath}`)
      }

      const data = await readFile(resolve(cssDir, relativePath))
      const dataUrl = `data:font/woff2;base64,${data.toString('base64')}`
      css = css.replaceAll(reference, `'${dataUrl}'`)
    }

    return css
      .replaceAll(sourceFamily, targetFamily)
      .replaceAll('font-display: swap', 'font-display: block')
  }))

  return styles.join('\n')
}

async function preparePage(page, fontCss) {
  await page.setContent(`
    <style>
      ${fontCss}
      * { box-sizing: border-box; }
      html, body, #scene {
        margin: 0;
        width: 100%;
        height: 100%;
        overflow: hidden;
      }
    </style>
    <style id="scene-styles"></style>
    <main id="scene"></main>
  `)

  const loaded = await page.evaluate(async () => {
    const [geist, noto] = await Promise.all([
      document.fonts.load("700 16px Geist", 'FinArch'),
      document.fonts.load("400 16px 'Noto Sans SC'", '记账报销统计财务工作台'),
    ])
    await document.fonts.ready
    return geist.length > 0 && noto.length > 0
  })

  if (!loaded) throw new Error('Pinned Geist and Noto Sans SC fonts failed to load.')
}

async function renderScene(page, { width, height, styles, markup, output }) {
  await page.setViewportSize({ width, height })
  await page.evaluate(({ styles: nextStyles, markup: nextMarkup }) => {
    document.querySelector('#scene-styles').textContent = nextStyles
    document.querySelector('#scene').innerHTML = nextMarkup
  }, { styles, markup })
  await page.evaluate(() => document.fonts.ready)
  await page.screenshot({
    path: resolve(publicDir, output),
    omitBackground: false,
    animations: 'disabled',
  })
}

async function renderSvg(page, source, output, width, height) {
  const svg = await readFile(resolve(publicDir, source), 'utf8')
  await renderScene(page, {
    width,
    height,
    output,
    markup: svg,
    styles: '#scene svg { display: block; width: 100%; height: 100%; }',
  })
}

function splashMarkup(scale) {
  const logoSize = 104 * scale
  const logoInnerSize = 80 * scale
  return {
    styles: `
      #scene {
        display: grid;
        place-items: center;
        color: #161A22;
        background: #F6F7F9;
        font-family: 'Geist', 'Noto Sans SC', sans-serif;
      }
      .content {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: ${20 * scale}px;
      }
      .mark {
        display: grid;
        width: ${logoSize}px;
        height: ${logoSize}px;
        place-items: center;
        overflow: hidden;
        border-radius: ${22 * scale}px;
        background: #161A22;
        box-shadow:
          0 ${10 * scale}px ${28 * scale}px rgba(17, 19, 24, 0.18),
          0 ${scale}px ${3 * scale}px rgba(17, 19, 24, 0.12);
      }
      .mark svg { width: ${logoInnerSize}px; height: ${logoInnerSize}px; }
      .brand {
        font-size: ${22 * scale}px;
        font-weight: 700;
        letter-spacing: -0.02em;
        line-height: 1;
      }
      .tagline {
        color: #5D6572;
        font-size: ${13 * scale}px;
        font-weight: 400;
        line-height: 1;
      }
    `,
    markup: `
      <div class="content">
        <div class="mark" aria-hidden="true">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
            <rect x="64" y="272" width="112" height="144" rx="24" fill="#6B7A99" />
            <rect x="200" y="192" width="112" height="224" rx="24" fill="#3D6BE8" />
            <rect x="336" y="96" width="112" height="320" rx="24" fill="#2FB37C" />
          </svg>
        </div>
        <div class="brand">FinArch</div>
        <div class="tagline">记账 · 报销 · 统计</div>
      </div>
    `,
  }
}

const fontCss = await loadBundledFontCss()
const browser = await chromium.launch({ headless: true })
const page = await browser.newPage({ deviceScaleFactor: 1 })

try {
  await preparePage(page, fontCss)

  for (const target of rasterTargets) {
    await renderSvg(page, target.source, target.output, target.width, target.height)
  }

  for (const { width, height, scale } of splashTargets) {
    const scene = splashMarkup(scale)
    await renderScene(page, {
      width,
      height,
      output: `splash-${width}x${height}.png`,
      ...scene,
    })
  }
} finally {
  await browser.close()
}
