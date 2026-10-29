import { chromium } from 'playwright-core'
import assert from 'node:assert/strict'

const browser = await chromium.launch({ executablePath: process.env.CHROME_PATH || undefined, headless: true, args: ['--no-sandbox', '--allow-file-access-from-files'] })
const text = '-- Escreva um comando SQL.\nSELECT 1;'
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 820 }, deviceScaleFactor: 1 })
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.addInitScript(text => {
    window.go = { main: { App: {
      LoadSettings: async () => ({ settings: { theme: 'light', maxRows: 200 }, warning: '' }),
      SaveSettings: async settings => settings,
      Drivers: async () => [{ kind: 'postgresql', name: 'PostgreSQL', language: 'sql' }],
      Profiles: async () => [],
      LoadWorkspace: async () => ({ warning: '', workspace: {
        version: 2, activeTabID: 1,
        scripts: [{ id: 'selection', title: 'Seleção', text, kind: 'postgresql', profileID: '', connectionName: 'Teste', database: '', updatedAt: '2026-10-29T12:00:00Z' }],
        openTabs: [{ id: 1, scriptID: 'selection', anchor: 0, head: 0 }],
      } }),
      SaveWorkspace: async () => {},
    } } }
  }, text)
  await page.goto(new URL('../dist/index.html', import.meta.url).href)
  const editor = page.locator('.cm-content')
  await editor.waitFor()
  await page.evaluate(() => document.fonts.ready)
  const content = () => editor.locator('.cm-line').allTextContents().then(lines => lines.join('\n'))
  async function selectWord() {
    await editor.focus(); await editor.press('Control+Home'); await editor.press('End'); await editor.press('ArrowLeft')
    for (let i = 0; i < 3; i++) await editor.press('Shift+ArrowLeft')
    assert.equal(await page.evaluate(() => window.getSelection().toString()), 'SQL')
  }
  async function changeTheme(theme) {
    const before = await page.evaluate(() => window.getSelection().toString())
    await page.getByRole('button', { name: 'Configurações', exact: true }).click()
    await page.getByRole('combobox', { name: 'Tema', exact: true }).selectOption(theme)
    await page.waitForFunction(theme => document.documentElement.dataset.theme === theme, theme)
    await page.getByRole('button', { name: 'Workspace', exact: true }).click()
    await editor.focus()
    assert.equal(await page.evaluate(() => window.getSelection().toString()), before, 'trocar tema perdeu a seleção')
  }
  // Compare o destaque renderizado com o mesmo trecho sem seleção.
  // Inspecionar apenas backgroundColor não detecta uma camada coberta pela linha ativa.
  async function visibleSelection(label, blurred = false) {
    await page.locator('.cm-selectionBackground').first().waitFor()
    if (blurred) await page.locator('.editor-toolbar button').filter({ hasText: 'Salvar' }).focus()
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    const samples = await page.evaluate(() => {
      const active = document.querySelector('.cm-activeLine').getBoundingClientRect()
      const marker = Array.from(document.querySelectorAll('.cm-selectionBackground')).map(el => el.getBoundingClientRect())
        .find(rect => rect.width > 10 && rect.height > 5 && rect.top < active.bottom && rect.bottom > active.top)
      if (!marker) throw new Error('Seleção ausente na linha ativa')
      return Array.from({ length: 5 }, (_, i) => ({ x: Math.floor(marker.left + 3 + (marker.width - 6) * i / 5), y: Math.floor(marker.top + 1) }))
    })
    const selected = await page.screenshot({ animations: 'disabled' })
    await editor.press('ArrowRight')
    if (blurred) await page.locator('.editor-toolbar button').filter({ hasText: 'Salvar' }).focus()
    const plain = await page.screenshot({ animations: 'disabled' })
    const differences = await page.evaluate(async ({ selected, plain, samples }) => {
      async function pixels(base64) {
        const img = new Image(); img.src = 'data:image/png;base64,' + base64; await img.decode()
        const canvas = document.createElement('canvas'); canvas.width = img.width; canvas.height = img.height
        const ctx = canvas.getContext('2d'); ctx.drawImage(img, 0, 0)
        return samples.map(({ x, y }) => Array.from(ctx.getImageData(x, y, 1, 1).data))
      }
      const a = await pixels(selected), b = await pixels(plain)
      return a.map((pixel, i) => pixel.slice(0, 3).reduce((sum, channel, j) => sum + Math.abs(channel - b[i][j]), 0))
    }, { selected: selected.toString('base64'), plain: plain.toString('base64'), samples })
    assert.ok(differences.filter(value => value >= 30).length >= 3, `${label}: seleção coberta; diferenças RGB ${differences}`)
    assert.equal(await content(), text, 'seleção alterou o texto')
    console.log(`Seleção visível: ${label}`)
  }
  for (const theme of ['light', 'dark']) {
    await selectWord()
    if (theme === 'dark') await changeTheme(theme)
    await visibleSelection(`${theme}: palavra SQL por teclado`)
    await selectWord(); await visibleSelection(`${theme}: seleção sem foco`, true)
    await editor.focus(); await editor.press('Control+Home'); await editor.press('Shift+ArrowDown'); await editor.press('Shift+End')
    assert.equal(await page.evaluate(() => window.getSelection().toString()), text)
    await visibleSelection(`${theme}: múltiplas linhas`)
    await editor.press('Control+a'); assert.equal(await page.evaluate(() => window.getSelection().toString()), text)
    await visibleSelection(`${theme}: Ctrl+A`)
    await editor.press('Control+Home')
    const bounds = await editor.locator('.cm-line').first().evaluate(line => {
      const walker = document.createTreeWalker(line, NodeFilter.SHOW_TEXT)
      while (walker.nextNode()) {
        const node = walker.currentNode, index = node.textContent.indexOf('SQL')
        if (index < 0) continue
        const range = document.createRange(); range.setStart(node, index); range.setEnd(node, index + 3)
        return range.getBoundingClientRect().toJSON()
      }
      throw new Error('Palavra SQL ausente')
    })
    await page.mouse.move(bounds.left + .2, bounds.top + bounds.height / 2); await page.mouse.down()
    await page.mouse.move(bounds.right - .2, bounds.top + bounds.height / 2, { steps: 6 }); await page.mouse.up()
    assert.equal(await page.evaluate(() => window.getSelection().toString()), 'SQL')
    await visibleSelection(`${theme}: palavra SQL por mouse`)
  }
  assert.deepEqual(errors, [])
} finally { await browser.close() }
