import { describe, expect, it } from 'vitest'
import { cycleIndex, nextTab, tabFromKey } from './keyboard'

describe('keyboard helpers', () => {
  it('cycles tabs with wraparound', () => {
    expect(nextTab('dashboard', 1)).toBe('chat')
    expect(nextTab('dashboard', -1)).toBe('backtest')
    expect(nextTab('backtest', 1)).toBe('dashboard')
  })

  it('maps numeric keys to tabs', () => {
    expect(tabFromKey('1')).toBe('dashboard')
    expect(tabFromKey('4')).toBe('backtest')
    expect(tabFromKey('9')).toBeNull()
  })

  it('cycles filter indexes', () => {
    expect(cycleIndex(0, 3)).toBe(1)
    expect(cycleIndex(2, 3)).toBe(0)
    expect(cycleIndex(0, 0)).toBe(0)
  })
})
