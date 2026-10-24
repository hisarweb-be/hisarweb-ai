/**
 * Version stamp shown in every footer: `Versie:<version>:DD-MM-YY:HH:MM`, the
 * build moment in Brussels time (house rule shared with the other HisarWeb
 * projects). rsbuild.config.ts formats it once at build time and bakes the
 * finished text into the bundle as `__BUILD_STAMP__`, so every visitor sees
 * the same text whatever their own time zone is.
 */
export function formatBuildStamp(version: string, moment: Date): string {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-GB', {
      timeZone: 'Europe/Brussels',
      day: '2-digit',
      month: '2-digit',
      year: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    })
      .formatToParts(moment)
      .map((part) => [part.type, part.value])
  )
  const cleanVersion = version.trim().replace(/^v/, '') || '0.0.0'
  return `Versie:${cleanVersion}:${parts.day}-${parts.month}-${parts.year}:${parts.hour}:${parts.minute}`
}

declare const __BUILD_STAMP__: string | undefined

// Empty in tests and tools that bundle without the rsbuild define.
export const BUILD_STAMP: string =
  typeof __BUILD_STAMP__ === 'string' ? __BUILD_STAMP__ : ''
