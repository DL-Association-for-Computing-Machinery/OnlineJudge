import assert from 'node:assert/strict'
import fs from 'node:fs/promises'
import path from 'node:path'
import console from 'node:console'

// Only the owner's isolated loopback fixture and synthetic code enter this run.
export async function runPreviewChecks(browser, info, output) {
  const context = await browser.newContext({ ignoreHTTPSErrors: true, viewport: { width: 1440, height: 1000 } })
  const page = await context.newPage()
  page.setDefaultTimeout(6000)
  let phase = 'identity gate'
  const posts = [], errors = [], screenshots = []
  page.on('request', request => { if (request.method() === 'POST') posts.push(new URL(request.url()).pathname) })
  page.on('pageerror', error => errors.push(error.name))
  await context.route('**/*', route => new URL(route.request().url()).origin === info.origin ? route.continue() : route.abort())
  const editor = () => page.getByRole('textbox', { name: '代码编辑器', exact: true })
  const shot = async name => {
    const image = path.join(output, name + '.png')
    await page.screenshot({ path: image, fullPage: true })
    screenshots.push(image)
  }
  try {
    // The editor must not accept work while its identity namespace is unsettled.
    let releaseRead
    const readGate = new Promise(resolve => { releaseRead = resolve })
    await page.route('**/api/v1/session', async route => { await readGate; await route.continue() })
    await page.goto(info.origin + '/preview/problems/P1001', { waitUntil: 'domcontentloaded' })
    await page.getByText('正在核对账号状态，核对完成后即可编辑。', { exact: true }).waitFor()
    assert.equal(await page.locator('.cm-content').count(), 0, 'No editable document before identity settles')
    releaseRead()
    await editor().waitFor()
    await page.unroute('**/api/v1/session')
    assert(await page.getByRole('button', { name: '提交代码', exact: true }).isDisabled(), 'Preview cannot submit code')
    assert.equal(await page.locator('.site-header').getByRole('link', { name: '题库', exact: true }).getAttribute('aria-current'), 'location')
    await page.getByLabel('语言预览', { exact: true }).focus()
    await page.keyboard.press('Tab')
    assert(await page.getByRole('button', { name: '上传代码', exact: true }).evaluate(element => element === document.activeElement), 'No hidden file input in the Tab order')
    await page.keyboard.press('Tab')
    assert(await page.getByRole('button', { name: '下载代码', exact: true }).evaluate(element => element === document.activeElement))
    await page.keyboard.press('Tab')
    assert(await editor().evaluate(element => element === document.activeElement))
    assert(await page.evaluate(() => document.documentElement.scrollHeight <= innerHeight), 'Workspace fits the desktop viewport vertically')
    await shot('desktop-workspace')

    phase = 'keyboard resize'
    const separator = page.getByRole('separator', { name: '调整题面宽度' })
    await separator.focus()
    await separator.press('ArrowRight')
    assert.equal(await separator.getAttribute('aria-valuenow'), '48', 'Keyboard resizes the reading pane')
    await separator.press('Home')
    assert.equal(await separator.getAttribute('aria-valuenow'), '30')
    await separator.press('End')
    assert.equal(await separator.getAttribute('aria-valuenow'), '65')

    phase = 'navigation retention'
    await editor().fill('// synthetic initial edit\nint main() { return 0; }')
    await page.locator('.site-header').getByRole('link', { name: '题库', exact: true }).click()
    await page.getByRole('heading', { name: '题库', exact: true }).waitFor()
    await page.getByRole('link', { name: 'A+B Problem', exact: true }).click()
    await editor().waitFor()
    assert((await editor().innerText()).includes('synthetic initial edit'), 'SPA navigation preserves temporary code')

    // Delay a real synthetic File read, unmount it, and start a new edit.
    phase = 'late file read'
    await page.evaluate(() => {
      window.__previewOriginalRead = File.prototype.arrayBuffer
      File.prototype.arrayBuffer = function () {
        const file = this
        return new Promise(resolve => {
          window.__releasePreviewRead = async () => resolve(await window.__previewOriginalRead.call(file))
        })
      }
    })
    page.once('dialog', dialog => dialog.accept())
    await page.getByLabel('选择预览代码文件', { exact: true }).setInputFiles({ name: 'preview.cpp', mimeType: 'text/plain', buffer: Buffer.from('// stale upload must not win') })
    await page.goBack()
    await page.getByRole('link', { name: 'A+B Problem', exact: true }).click()
    await editor().fill('// synthetic newest edit\nint main() { return 1; }')
    await page.evaluate(async () => {
      await window.__releasePreviewRead()
      File.prototype.arrayBuffer = window.__previewOriginalRead
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    })
    assert((await editor().innerText()).includes('synthetic newest edit'), 'Unmounted old file read cannot overwrite a new editor')

    phase = 'newest file selection wins'
    await page.evaluate(() => {
      window.__previewReads = {}
      File.prototype.arrayBuffer = function () {
        const file = this
        return new Promise(resolve => { window.__previewReads[file.name] = async () => resolve(await window.__previewOriginalRead.call(file)) })
      }
    })
    for (const name of ['first', 'second']) {
      page.once('dialog', dialog => dialog.accept())
      await page.getByLabel('选择预览代码文件', { exact: true }).setInputFiles({ name: name + '.cpp', mimeType: 'text/plain', buffer: Buffer.from('// synthetic ' + name + ' selection') })
    }
    await page.evaluate(async () => {
      await window.__previewReads['first.cpp']()
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    })
    assert((await editor().innerText()).includes('synthetic newest edit'), 'Older selection cannot commit ahead of the newest pending file')
    await page.evaluate(async () => {
      await window.__previewReads['second.cpp']()
      File.prototype.arrayBuffer = window.__previewOriginalRead
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    })
    assert((await editor().innerText()).includes('synthetic second selection'), 'Newest selected file is accepted')
    for (const invalidOldFile of [false, true]) {
      await page.evaluate(() => {
        window.__previewReads = {}
        File.prototype.arrayBuffer = function () {
          const file = this
          return new Promise(resolve => { window.__previewReads[file.name] = async () => resolve(await window.__previewOriginalRead.call(file)) })
        }
      })
      for (const name of ['first', 'second']) {
        page.once('dialog', dialog => dialog.accept())
        const buffer = name === 'first' && invalidOldFile ? Buffer.from([0xc3, 0x28]) : Buffer.from('// synthetic reverse ' + name)
        await page.getByLabel('选择预览代码文件', { exact: true }).setInputFiles({ name: name + '.cpp', mimeType: 'text/plain', buffer })
      }
      for (const name of ['second', 'first']) {
        await page.evaluate(async name => {
          await window.__previewReads[name + '.cpp']()
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        }, name)
        assert((await editor().innerText()).includes('synthetic reverse second'), 'Older completion or error cannot overwrite newer success')
        await page.getByText('文件已载入预览编辑器，尚未提交。', { exact: true }).waitFor()
      }
      await page.evaluate(() => { File.prototype.arrayBuffer = window.__previewOriginalRead })
    }

    // Import/export must preserve the source's original bytes until edited.
    phase = 'source import and export'
    const raw = '\ufeff// synthetic 中文 e\u0301\r\n\tint main() {}\r\n'
    page.once('dialog', dialog => dialog.accept())
    await page.getByLabel('选择预览代码文件', { exact: true }).setInputFiles({ name: 'preview.cpp', mimeType: 'text/plain', buffer: Buffer.from(raw) })
    await page.getByText('文件已载入预览编辑器，尚未提交。', { exact: true }).waitFor()
    const downloadPromise = page.waitForEvent('download')
    await page.getByRole('button', { name: '下载代码', exact: true }).click()
    const download = await downloadPromise
    const file = path.join(output, 'synthetic-preview-download.cpp')
    await download.saveAs(file)
    assert.equal(await fs.readFile(file, 'utf8'), raw, 'Download preserves raw UTF-8, Unicode and CRLF')
    page.once('dialog', dialog => dialog.accept())
    await page.getByLabel('选择预览代码文件', { exact: true }).setInputFiles({ name: 'invalid.cpp', mimeType: 'application/octet-stream', buffer: Buffer.from([0xc3, 0x28]) })
    await page.getByText('文件无法作为 UTF-8 文本载入，或超过 128 KiB。当前代码已保留。', { exact: true }).waitFor()
    assert((await editor().innerText()).includes('synthetic 中文'), 'Rejected input leaves the current source intact')

    // The beforeunload guard belongs to the documents, even on the library page.
    phase = 'global beforeunload'
    await page.locator('.site-header').getByRole('link', { name: '题库', exact: true }).click()
    const dialogPromise = page.waitForEvent('dialog')
    const reload = page.reload().catch(() => null)
    const dialog = await dialogPromise
    assert.equal(dialog.type(), 'beforeunload')
    await dialog.dismiss()
    await reload
    await page.getByRole('heading', { name: '题库', exact: true }).waitFor()
    phase = 'library filtering and pagination'
    await page.getByLabel('搜索示例题目', { exact: true }).fill('P1004')
    assert.equal(await page.locator('.problem-table tbody tr').count(), 1)
    assert((await page.locator('.problem-table tbody').innerText()).includes('括号匹配'))
    await page.getByLabel('搜索示例题目', { exact: true }).fill('no such synthetic title')
    await page.getByText('没有匹配的示例题目', { exact: true }).waitFor()
    await page.getByRole('button', { name: '清除搜索与筛选', exact: true }).click()
    await page.locator('.filters summary').click()
    await page.getByRole('combobox', { name: '难度', exact: true }).selectOption('中等')
    await page.waitForFunction(() => {
      const text = document.querySelector('.problem-table tbody')?.textContent || ''
      return text.includes('最长递增子序列') && !text.includes('A+B Problem')
    })
    assert((await page.locator('.problem-table tbody').innerText()).includes('最长递增子序列'))
    assert(!(await page.locator('.problem-table tbody').innerText()).includes('A+B Problem'))
    await page.getByRole('button', { name: '清除筛选', exact: true }).click()
    await page.getByRole('link', { name: 'A+B Problem', exact: true }).waitFor()
    await page.locator('.filters summary').click()
    await page.getByRole('button', { name: '下一页', exact: true }).click()
    await page.getByRole('link', { name: '最大值', exact: true }).waitFor()
    assert((await page.locator('.problem-table tbody').innerText()).includes('最大值'))
    await page.getByRole('button', { name: '上一页', exact: true }).click()
    await shot('desktop-library')
    await page.emulateMedia({ reducedMotion: 'reduce' })
    assert.equal(await page.getByRole('button', { name: '下一页', exact: true }).evaluate(element => getComputedStyle(element).transitionDuration), '0s')
    await page.setViewportSize({ width: 1280, height: 900 })
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Desktop library has no overflow')
    assert.deepEqual(posts, [], 'Preview never sends a POST or code to the backend')
    assert.deepEqual(errors, [], 'Preview has no uncaught application errors')
    return screenshots
  } catch (error) {
    console.error('Preview QA failed at ' + phase + ' (' + error.name + ')')
    await page.screenshot({ path: path.join(output, 'preview-failure.png'), fullPage: true }).catch(() => {})
    throw error
  } finally { await context.close() }
}
