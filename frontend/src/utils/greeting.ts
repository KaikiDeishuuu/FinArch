export type GreetingLocale = 'zh-CN' | 'en'

const TRAILING_PUNCTUATION_REGEX = /[，,。.!?！？]+$/g

function stripTrailingPunctuation(text: string): string {
  return text.replace(TRAILING_PUNCTUATION_REGEX, '').trim()
}

export function formatGreeting({
  locale,
  greeting,
  message,
  username,
}: {
  locale: GreetingLocale;
  greeting: string;
  message: string;
  username?: string;
}) {
  const safeGreeting = stripTrailingPunctuation(greeting)
  const safeMessage = stripTrailingPunctuation(message)
  const safeUsername = username ? stripTrailingPunctuation(username) : ''

  if (locale === 'zh-CN') {
    return `${safeGreeting}，${safeMessage}${safeUsername ? `，${safeUsername}` : ''}`
  }

  return `${safeGreeting}, ${safeMessage}${safeUsername ? `, ${safeUsername}` : ''}.`
}

export function normalizeGreetingLocale(language: string): GreetingLocale {
  return language.startsWith('zh') ? 'zh-CN' : 'en'
}
