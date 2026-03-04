import type { AxiosError } from 'axios'

export type ApiErrorCode =
  | 'username_taken'
  | 'email_taken'
  | 'invalid_token'
  | 'expired_token'
  | 'already_used'
  | 'not_authorized'
  | 'user_not_found'
  | 'resource_conflict'
  | 'email_not_verified'
  | 'internal_error'
  | 'unknown_error'

export const API_ERROR_MESSAGES: Record<ApiErrorCode, string> = {
  username_taken: 'This username is already taken. Please try another one.',
  email_taken: 'This email is already in use. Please try another one.',
  invalid_token: 'The link is invalid. Please request a new one.',
  expired_token: 'This link has expired. Please request a new one.',
  already_used: 'This action has already been completed.',
  not_authorized: 'You are not authorized to perform this action.',
  user_not_found: 'User not found.',
  resource_conflict: 'The request conflicts with existing data.',
  email_not_verified: 'Please verify your email before logging in.',
  internal_error: 'Something went wrong. Please try again.',
  unknown_error: 'Something went wrong. Please try again.',
}

export function getApiError(err: unknown): { code: string; message: string } {
  const axiosErr = err as AxiosError<{ error?: { code?: string; message?: string }; message?: string }>
  const code = axiosErr.response?.data?.error?.code ?? 'unknown_error'
  const message = axiosErr.response?.data?.error?.message ?? API_ERROR_MESSAGES[code as ApiErrorCode] ?? API_ERROR_MESSAGES.unknown_error
  return { code, message }
}
