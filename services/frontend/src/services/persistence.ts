/**
 * Session persistence.
 *
 * Everything the user chose — waypoints, boat, penalties, layer toggles, which view they were on,
 * and the routes they computed — is mirrored into localStorage so a reload, or coming back
 * tomorrow, puts them back exactly where they were.
 *
 * State is stored one key per value rather than as a single blob. A multi-model route carries
 * megabytes of isochrone geometry, and per-key storage means an oversized value can be shed on
 * its own without taking the small inputs and settings down with it.
 */

import { Dispatch, SetStateAction, useEffect, useRef, useState } from 'react';
import { MultiRouteResult, RouteResult } from '../types';
import { readPersisted, writePersisted, ShrinkFn } from './persistentStorage';

export { clearPersistedState } from './persistentStorage';

export interface PersistOptions<T> {
  /** Transforms a stored value before it becomes state, e.g. to expire a stale timestamp. */
  revive?: (stored: T) => T;
  /** Fallbacks tried in order if the full value will not fit. */
  shrink?: ShrinkFn<T>[];
}

/**
 * useState that survives a reload.
 *
 * Reads synchronously during the first render so the restored UI paints immediately rather than
 * flashing defaults, and writes whenever the value changes.
 */
export function usePersistedState<T>(
  key: string,
  initialValue: T | (() => T),
  options: PersistOptions<T> = {}
): [T, Dispatch<SetStateAction<T>>] {
  // Held in a ref so callers can pass inline option objects without retriggering the write effect.
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const [value, setValue] = useState<T>(() => {
    const fallback = () =>
      typeof initialValue === 'function' ? (initialValue as () => T)() : initialValue;

    const stored = readPersisted<T>(key);
    if (stored === undefined) return fallback();

    const { revive } = optionsRef.current;
    if (!revive) return stored;

    try {
      return revive(stored);
    } catch (err) {
      console.warn(`Could not revive persisted "${key}", using default:`, err);
      return fallback();
    }
  });

  useEffect(() => {
    writePersisted(key, value, optionsRef.current.shrink);
  }, [key, value]);

  return [value, setValue];
}

/**
 * Isochrone geometry dominates the size of a stored route — a point array per wave, per model —
 * and only feeds an optional map overlay, so it is the first thing sacrificed when storage is
 * full. The route itself, which the whole view is built around, survives.
 */
export const dropIsochrones: ShrinkFn<RouteResult | null> = (route) =>
  route ? { ...route, isochrones: [] } : route;

export const dropIsochronesFromAll: ShrinkFn<MultiRouteResult | null> = (routes) => {
  if (!routes) return routes;
  const trimmed: MultiRouteResult = {};
  for (const [modelId, route] of Object.entries(routes)) {
    trimmed[modelId] = { ...route, isochrones: [] };
  }
  return trimmed;
};

/** The app's departure-time format: a UTC "YYYY-MM-DDTHH:mm" string. */
export function nowDepartureValue(): string {
  return new Date().toISOString().slice(0, 16);
}

/**
 * A departure time points at a forecast. Restoring one that has already passed would show a route
 * built from weather that no longer exists, so it snaps forward to now. That also makes the
 * restored departure differ from the one the route was solved with, which trips the app's
 * isRouteOutdated check and highlights the Calculate button.
 */
export function reviveDepartureTime(saved: string): string {
  const savedMs = Date.parse(`${saved}:00Z`);
  if (!Number.isFinite(savedMs) || savedMs < Date.now()) {
    return nowDepartureValue();
  }
  return saved;
}
