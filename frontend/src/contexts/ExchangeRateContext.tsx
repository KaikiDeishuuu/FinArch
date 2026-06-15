import { useEffect, useState } from 'react'
import { fetchRates, FALLBACK_RATES } from '../utils/exchangeRates'
import type { RateCache } from '../utils/exchangeRates'
import { ExchangeRateContext } from './exchangeRateContextCore'
import type { ExchangeRateCtx } from './exchangeRateContextCore'

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
