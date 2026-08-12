/** Drop null entries that Wails v3 pointer-slice bindings may surface. */
export function compact<T>(items: (T | null | undefined)[] | null | undefined): T[] {
  if (!items) {
    return []
  }

  return items.filter((item): item is T => item != null)
}

/** Require a non-null binding result; throw so callers land in their catch. */
export function requireValue<T>(value: T | null | undefined, label = 'value'): T {
  if (value == null) {
    throw new Error(`unexpected empty ${label}`)
  }

  return value
}
