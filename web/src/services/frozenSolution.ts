export interface FrozenSolution { source: string; dependencies?: { name: string; source: string }[] }

const sourceLimit = 64 * 1024
const encoder = new TextEncoder()
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
const allowedDependencies = new Set(['lib/prime.hpp', 'lib/montgomery.hpp', 'lib/util.hpp', 'common/random.h'])
const object = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)
const invalid = () => new Error('无法核实冻结标程。')

function sourceBytes(value: unknown) {
  if (typeof value !== 'string' || value.length === 0 || value.length > sourceLimit) throw invalid()
  const raw = encoder.encode(value)
  if (raw.byteLength > sourceLimit || decoder.decode(raw) !== value) throw invalid()
  return raw.byteLength
}

export function decodeFrozenSolution(solution: unknown): FrozenSolution {
  if (!object(solution) || Object.keys(solution).length !== ('dependencies' in solution ? 2 : 1)) throw invalid()
  let bytes = sourceBytes(solution.source)
  if ('dependencies' in solution) {
    if (!Array.isArray(solution.dependencies) || solution.dependencies.length < 1 || solution.dependencies.length > allowedDependencies.size) throw invalid()
    const seen = new Set<string>()
    for (const dep of solution.dependencies) {
      if (!object(dep) || Object.keys(dep).length !== 2 || typeof dep.name !== 'string' || !allowedDependencies.has(dep.name) || seen.has(dep.name)) throw invalid()
      seen.add(dep.name)
      bytes += sourceBytes(dep.source)
      if (bytes > sourceLimit) throw invalid()
    }
  }
  return solution as unknown as FrozenSolution
}
