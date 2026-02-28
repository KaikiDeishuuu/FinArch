import { createContext, useContext, useEffect, useState } from 'react'
import { fetchRates, FALLBACK_RATES } from '../utils/exchangeRates'
import type { RateCache } from '../utils/exchangeRates'

interface ExchangeRateCtx {
  rates: Record<string, number>
  /** ISO date of rates, empty string means fallback/offline */
  rateDate: string
  loading: boolean
}

const ExchangeRateContext = createContext<ExchangeRateCtx>({
  rates: FALLBACK_RATES,
  rateDate: '',
  loading: true,
})

export function ExchangeRateProvider({ children }: { children: React.ReactNode }) {
  const [ctx, setCtx] = useState<ExchangeRateCtx>({
    rates: FALLBACK_RATES,
    rateDate: '',
    loading: true,
  })

  useEffect(() => {
    fetchRates().then((cache: RateCache) => {
      setCtx({ rates: cache.rates, rateDate: cache.date, loading: false })
    })
  }, [])

  return (
    <ExchangeRateContext.Provider value={ctx}>
      {children}
    </ExchangeRateContext.Provider>
  )
}

export function useExchangeRates() {
  return useContext(ExchangeRateContext)
}
