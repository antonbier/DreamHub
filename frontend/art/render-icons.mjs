// Renders the app icons in ../public/icons (icon.svg, icon-maskable.svg)
// to the PNG sizes the web app manifest and iOS ask for, through headless
// Chromium (playwright-core, not a dependency of the app — install it next
// to this script or point NODE_PATH at it). Usage:
//   node render-icons.mjs <chromium executable>
import { chromium } from 'playwright-core'
import { readFileSync } from 'fs'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'icons')
const [exe] = process.argv.slice(2)
const jobs = [
  ['icon.svg', 192, 'icon-192.png', false],
  ['icon.svg', 512, 'icon-512.png', false],
  ['icon-maskable.svg', 512, 'icon-maskable-512.png', false],
  // iOS puts its own rounded corners on an opaque square.
  ['icon-maskable.svg', 180, 'apple-touch-icon.png', true],
]
const browser = await chromium.launch({ executablePath: exe })
for (const [src, size, out, opaque] of jobs) {
  const svg = readFileSync(join(dir, src), 'utf8').replace(/<!--[\s\S]*?-->\s*/, '')
  const page = await browser.newPage({ viewport: { width: size, height: size } })
  await page.setContent(`<style>html,body{margin:0;background:${opaque ? '#161A1F' : 'transparent'}}svg{display:block;width:${size}px;height:${size}px}</style>${svg}`)
  await page.locator('svg').screenshot({ path: join(dir, out), omitBackground: !opaque })
  await page.close()
  console.log(out)
}
await browser.close()
