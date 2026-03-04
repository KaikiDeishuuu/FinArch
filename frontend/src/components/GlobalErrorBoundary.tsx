import { Component, type ErrorInfo, type ReactNode } from 'react'

type Props = { children: ReactNode }
type State = { hasError: boolean }

export default class GlobalErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    if (import.meta.env.DEV) {
      console.error('GlobalErrorBoundary', error, info)
    }
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen flex items-center justify-center p-6 text-center">
          <div>
            <h1 className="text-xl font-semibold">Something went wrong.</h1>
            <p className="mt-2 text-sm text-gray-500">Please refresh and try again.</p>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
