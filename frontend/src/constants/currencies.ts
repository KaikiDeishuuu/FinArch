export interface CurrencyMeta {
  code: string
  en: string
  zh: string
}

export const SUPPORTED_CURRENCIES: CurrencyMeta[] = [
  { code: 'CNY', en: 'Chinese Yuan', zh: '人民币' },
  { code: 'USD', en: 'US Dollar', zh: '美元' },
  { code: 'EUR', en: 'Euro', zh: '欧元' },
  { code: 'GBP', en: 'British Pound', zh: '英镑' },
  { code: 'HKD', en: 'Hong Kong Dollar', zh: '港元' },
  { code: 'JPY', en: 'Japanese Yen', zh: '日元' },
  { code: 'SGD', en: 'Singapore Dollar', zh: '新加坡元' },
  { code: 'AUD', en: 'Australian Dollar', zh: '澳元' },
  { code: 'CAD', en: 'Canadian Dollar', zh: '加元' },
  { code: 'KRW', en: 'South Korean Won', zh: '韩元' },
  { code: 'CHF', en: 'Swiss Franc', zh: '瑞士法郎' },
  { code: 'INR', en: 'Indian Rupee', zh: '印度卢比' },
  { code: 'THB', en: 'Thai Baht', zh: '泰铢' },
  { code: 'MYR', en: 'Malaysian Ringgit', zh: '马来西亚林吉特' },
]

export const CURRENCY_SYMBOLS: Record<string, string> = {
  CNY: '¥', USD: '$', EUR: '€', GBP: '£', HKD: 'HK$', JPY: '¥', SGD: 'S$',
  AUD: 'A$', CAD: 'C$', KRW: '₩', CHF: 'CHF', INR: '₹', THB: '฿', MYR: 'RM',
}
