import { describe, expect, it } from 'vitest'
import { normalizeCategory } from './category'

describe('category wire contract', () => {
  it('normalizes historical field names at one boundary', () => {
    expect(normalizeCategory({ ID: 8, ParentID: 2, Level: 2, Name: '家具', Status: 'DISABLED', Sort: 0 })).toEqual({ id: 8, parent_id: 2, merchant_id: undefined, level: 2, name: '家具', status: 'DISABLED', sort: 0 })
  })
  it('prefers current fields, preserving null root parent and zero sort', () => {
    expect(normalizeCategory({ id: 1, ID: 9, parent_id: null, ParentID: 8, name: 'root', Name: 'legacy', level: 1, sort: 0, Sort: 5 })).toMatchObject({ id: 1, parent_id: null, name: 'root', sort: 0 })
  })
})
