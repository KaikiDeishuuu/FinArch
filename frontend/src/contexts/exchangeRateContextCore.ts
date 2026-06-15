import { createContext } from 'react'
import { FALLBACK_RATES } from '../utils/exchangeRates'

export interface ExchangeRateCtx {
  rates: Record<string, number>
  rateDate: string
  loading: boolean
}

export const ExchangeRateContext = createContext<ExchangeRateCtx>({
  rates: FALLBACK_RATES,
  rateDate: '',
  loading: true,
})
