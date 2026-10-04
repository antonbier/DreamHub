// Renders plugin-icon.svg to ../src/plugin.png with a transparent ground,
// through headless Chromium (playwright-core) so the text uses the same
// bundled Source Sans 3 as the web UI. Usage:
//   node render-icon.mjs <chromium executable> <source-sans-3-700.woff2> [scale]
import { chromium } from 'playwright-core'
import { readFileSync } from 'fs'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'

const here = dirname(fileURLToPath(import.meta.url))
const [exe, font, scale = '1'] = process.argv.slice(2)
const svg = readFileSync(join(here, 'plugin-icon.svg'), 'utf8')
const fontData = readFileSync(font).toString('base64')
const html = `<style>@font-face{font-family:"Source Sans 3";font-weight:700;src:url(data:font/woff2;base64,${fontData})}
html,body{margin:0;background:transparent}svg{display:block}</style>${svg}`

const browser = await chromium.launch({ executablePath: exe })
const page = await browser.newPage({ viewport: { width: 100, height: 40 }, deviceScaleFactor: Number(scale) })
await page.setContent(html)
await page.evaluate(() => document.fonts.load('700 13px "Source Sans 3"'))
const out = scale === '1' ? join(here, '..', 'src', 'plugin.png') : join(here, `plugin-icon@${scale}x.png`)
await page.locator('svg').screenshot({ path: out, omitBackground: true })
await browser.close()
console.log(out)
