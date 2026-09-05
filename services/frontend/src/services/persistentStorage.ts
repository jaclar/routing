/**
 * The storage engine behind session persistence: namespacing, JSON safety, and shedding
 * oversized values when the browser runs out of room.
 *
 * Deliberately free of React so it can be exercised on its own.
 */

const STORAGE_PREFIX = 'sailboat.routing.v1.';

/** Progressively smaller renderings of a value, tried in order when storage is full. */
export type ShrinkFn<T> = (value: T) => T;

/** Reads a stored value, or undefined when absent, unreadable, or unparseable. */
export function readPersisted<T>(key: string): T | undefined {
  try {
    const raw = storage()?.getItem(STORAGE_PREFIX + key);
    if (raw === null || raw === undefined) return undefined;
    return JSON.parse(raw) as T;
  } catch (err) {
    // Corrupted entry, or storage blocked by the browser. Fall back to the caller's default
    // rather than breaking startup.
    console.warn(`Ignoring unreadable persisted value for "${key}":`, err);
    return undefined;
  }
}

/**
 * Writes a value, falling back through `shrink` if the browser rejects it for quota.
 *
 * Returns how the write resolved, which callers may log but need not act on: persistence is a
 * convenience, and failing to store must never break the session in progress.
 */
export function writePersisted<T>(
  key: string,
  value: T,
  shrink?: ShrinkFn<T>[]
): 'stored' | 'stored-reduced' | 'dropped' {
  const store = storage();
  if (!store) return 'dropped';

  const attempts: T[] = [value];
  for (const fn of shrink ?? []) {
    try {
      attempts.push(fn(value));
    } catch {
      // A shrink that cannot produce a value simply is not offered as a fallback.
    }
  }

  for (let i = 0; i < attempts.length; i++) {
    try {
      store.setItem(STORAGE_PREFIX + key, JSON.stringify(attempts[i]));
      return i === 0 ? 'stored' : 'stored-reduced';
    } catch (err) {
      if (!isQuotaExceeded(err)) {
        console.warn(`Could not persist "${key}":`, err);
        return 'dropped';
      }
    }
  }

  // Nothing fit. Remove the key rather than leave a stale oversized value behind; that also
  // frees room for the inputs and settings, which are the ones worth keeping.
  try {
    store.removeItem(STORAGE_PREFIX + key);
  } catch {
    // Storage is unavailable entirely; there is nothing further to do.
  }
  return 'dropped';
}

/** Removes every persisted value, returning the app to a first-visit state. */
export function clearPersistedState(): void {
  const store = storage();
  if (!store) return;
  try {
    // Collected first via the index API, since removing while iterating shifts the indices.
    const doomed: string[] = [];
    for (let i = 0; i < store.length; i++) {
      const k = store.key(i);
      if (k?.startsWith(STORAGE_PREFIX)) doomed.push(k);
    }
    doomed.forEach((k) => store.removeItem(k));
  } catch (err) {
    console.warn('Could not clear persisted state:', err);
  }
}

export function isQuotaExceeded(err: unknown): boolean {
  if (typeof DOMException !== 'undefined' && err instanceof DOMException) {
    return (
      err.name === 'QuotaExceededError' ||
      err.name === 'NS_ERROR_DOM_QUOTA_REACHED' || // Firefox
      err.code === 22 ||
      err.code === 1014
    );
  }
  // Some environments throw a plain Error for quota; match on the conventional names.
  const name = (err as { name?: string } | null)?.name;
  return name === 'QuotaExceededError' || name === 'NS_ERROR_DOM_QUOTA_REACHED';
}

/** localStorage, or null where it is unavailable (private modes, embedded webviews). */
function storage(): Storage | null {
  try {
    return typeof window !== 'undefined' && window.localStorage ? window.localStorage : null;
  } catch {
    return null;
  }
}
