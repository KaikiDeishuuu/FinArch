import type { AppMode } from '../contexts/ModeContext'

export interface ChartPalette {
    income: string
    expense: string
    secondary: string
}

/**
 * Returns mode-aware chart colors.
 * WORK: blue/red corporate tones
 * LIFE: emerald/amber warm tones
 */
export function getModeChartPalette(mode: AppMode): ChartPalette {
    if (mode === 'life') {
        return {
            income: '#10b981',   // emerald-500
            expense: '#f59e0b',  // amber-500
            secondary: '#14b8a6', // teal-500
        }
    }
    return {
        income: '#3b82f6',    // blue-500
        expense: '#ef4444',   // red-500
        secondary: '#6366f1', // indigo-500
    }
}
