/**
 * KEY=VALUE helpers shared by the provider dialogs (Codex `env_http_headers`,
 * Hermes `extra_headers`).
 */

/** Render a string map as editable KEY=VALUE lines. */
export function formatKeyValueLines(
  values?: Partial<Record<string, string>> | null
): string {
  if (!values) return ''
  return Object.entries(values)
    .filter((entry): entry is [string, string] => entry[1] !== undefined)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

/** Parse KEY=VALUE lines; empty input returns null. */
export function parseKeyValueLines(
  text: string
): Record<string, string> | null {
  const entries = text
    .split('\n')
    .map(line => line.trim())
    .filter(Boolean)
    .map(line => {
      const index = line.indexOf('=')
      if (index <= 0) return null
      return [line.slice(0, index).trim(), line.slice(index + 1)] as const
    })
    .filter((entry): entry is readonly [string, string] => entry !== null)
  if (entries.length === 0) return null
  return Object.fromEntries(entries)
}
