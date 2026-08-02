// errorText turns whatever a wails binding rejected with into one readable line.
// A bound method rejects with the Go error string, but a transport failure rejects
// with an Error object, so both shapes are normalised in one place instead of every
// catch block spelling out String(e).
export function errorText(e: unknown): string {
  if (typeof e === 'string') {
    return e
  }

  if (e instanceof Error) {
    return e.message
  }

  return String(e)
}
