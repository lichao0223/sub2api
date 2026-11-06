import { describe, expect, it } from 'vitest'
import { organizationDirectory, tokenShare } from '../organizations'
import type { AnalyticsDirectoryState, AnalyticsDepartment } from '@/api/admin/dataAnalytics'

describe('analytics organization scope', () => {
  it('folds fourth-level directory members into all three visible ancestors and preserves historical organizations', () => {
    const orgs = ['company','center','department','team'].map((id,index)=>({
      id, parent:index ? ['company','center','department'][index-1]! : '',
      name:id, path:['company','center','department','team'].slice(0,index+1),
      names:['company','center','department','team'].slice(0,index+1), level:index+1,
    }))
    const state = {
      directory:{orgs,people:[{id:'p1',name:'甲',org:'team',active:true},{id:'p2',name:'乙',org:'missing',active:true}]},
      identities:[{user_id:1,person_id:'p1'},{user_id:2,person_id:'p2'},{user_id:3,person_id:''}],
    } as unknown as AnalyticsDirectoryState
    const historical = {id:'old',parent:'center',name:'历史部门',level:3} as AnalyticsDepartment
    const view = organizationDirectory(state,[historical])
    expect([...view.nodes.keys()]).toEqual(['company','center','department','old'])
    expect(view.nodes.get('old')?.path).toEqual(['company','center','old'])
    expect(view.counts.get('company')).toBe(1)
    expect(view.counts.get('center')).toBe(1)
    expect(view.counts.get('department')).toBe(1)
    expect(view.counts.has('team')).toBe(false)
    expect(view.pending).toBe(2)
  })
  it('calculates bounded display percentages without converting large token counts to floats', () => {
    expect(tokenShare('9007199254740993','18014398509481986')).toBe(50)
    expect(tokenShare('1','0')).toBe(0)
    expect(tokenShare('10','5')).toBe(100)
  })
})
