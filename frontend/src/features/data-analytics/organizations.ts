import type { AnalyticsDirectoryState, AnalyticsDepartment, AnalyticsOrg } from '@/api/admin/dataAnalytics'

export function organizationDirectory(state: AnalyticsDirectoryState | undefined, departments: AnalyticsDepartment[] = []) {
  const all = new Map((state?.directory.orgs || []).map(org => [org.id, org]))
  const nodes = new Map<string, AnalyticsOrg>()
  for (const org of all.values()) if (org.level <= 3) nodes.set(org.id, org)
  // Retained usage can refer to an organization missing from the latest directory.
  for (const department of departments) {
    if (!nodes.has(department.id) && department.id !== '0') {
      const parent = nodes.get(department.parent)
      nodes.set(department.id, {
        ...department,
        path: [...(parent?.path || []), department.id],
        names: [...(parent?.names || []), department.name],
      })
    }
  }
  const counts = new Map<string, number>()
  for (const person of state?.directory.people || []) {
    for (const id of all.get(person.org)?.path.slice(0, 3) || []) {
      counts.set(id, (counts.get(id) || 0) + 1)
    }
  }
  const people = new Map((state?.directory.people || []).map(person => [person.id, person]))
  const pending = (state?.identities || []).filter(identity => {
    const person = people.get(identity.person_id)
    return !person || !all.has(person.org)
  }).length
  return { nodes, counts, pending }
}

export function tokenShare(value: string | undefined, total: string | undefined): number {
  const numerator = BigInt((value || '0').split('.')[0] || '0')
  const denominator = BigInt((total || '0').split('.')[0] || '0')
  if (denominator <= 0n || numerator <= 0n) return 0
  return Math.min(100, Number(numerator * 10000n / denominator) / 100)
}
