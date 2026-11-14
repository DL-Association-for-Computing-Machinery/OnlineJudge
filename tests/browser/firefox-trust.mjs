import fs from 'node:fs/promises'
import net from 'node:net'
import path from 'node:path'

// Use a private policy and block non-fixture traffic without intercepting the
// actual HTTPS requests or changing their Origin and Fetch Metadata headers.
export async function firefoxTrust(firefox, output) {
  const policy = path.join(output, 'firefox-policy.json')
  await fs.writeFile(policy, JSON.stringify({ policies: { Certificates: {
    ImportEnterpriseRoots: false, Install: [process.env.OJ_BROWSER_CA_FILE],
  } } }), { flag: 'wx' })
  const deny = net.createServer(socket => socket.destroy())
  await new Promise((resolve, reject) => { deny.once('error', reject); deny.listen(0, '127.0.0.1', resolve) })
  const proxy = { server: `http://127.0.0.1:${deny.address().port}`, bypass: '127.0.0.1' }
  return {
    launch(trusted) {
      const env = { ...process.env }
      delete env.PLAYWRIGHT_FIREFOX_POLICIES_JSON
      if (trusted) env.PLAYWRIGHT_FIREFOX_POLICIES_JSON = policy
      return firefox.launch({ executablePath: process.env.OJ_FIREFOX_EXECUTABLE, headless: true, proxy,
        env, firefoxUserPrefs: { 'security.enterprise_roots.enabled': false } })
    },
    close: () => new Promise(resolve => deny.close(resolve)),
  }
}
