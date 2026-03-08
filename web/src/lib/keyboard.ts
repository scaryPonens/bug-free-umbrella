import type { TabKey } from '../types/events'

export const TABS: TabKey[] = ['dashboard', 'chat', 'signals', 'backtest']

export const SIGNAL_SYMBOL_OPTIONS = ['ALL', 'BTC', 'ETH', 'SOL', 'XRP', 'ADA', 'DOGE', 'DOT', 'AVAX', 'LINK', 'MATIC']
export const SIGNAL_RISK_OPTIONS = ['ALL', '1', '2', '3', '4', '5']
export const SIGNAL_INDICATOR_OPTIONS = [
  'ALL',
  'rsi',
  'macd',
  'bollinger',
  'volume_zscore',
  'ml_logreg_up4h',
  'ml_xgboost_up4h',
  'ml_ensemble_up4h',
  'fund_sentiment_composite',
]

export function nextTab(current: TabKey, direction: 1 | -1): TabKey {
  const index = TABS.indexOf(current)
  if (index < 0) {
    return 'dashboard'
  }
  const next = (index + direction + TABS.length) % TABS.length
  return TABS[next]
}

export function tabFromKey(key: string): TabKey | null {
  if (key === '1') {
    return 'dashboard'
  }
  if (key === '2') {
    return 'chat'
  }
  if (key === '3') {
    return 'signals'
  }
  if (key === '4') {
    return 'backtest'
  }
  return null
}

export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }
  const tag = target.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || target.isContentEditable) {
    return true
  }
  return false
}

export function cycleIndex(current: number, length: number): number {
  if (length <= 0) {
    return 0
  }
  return (current + 1) % length
}
