import { useContext } from 'react'
import { ExchangeRateContext } from '../contexts/exchangeRateContextCore'

export function useExchangeRates() {
  return useContext(ExchangeRateContext)
}
