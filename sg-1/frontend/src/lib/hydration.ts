"use client";
import { useSyncExternalStore } from "react";

const subscribe = () => () => {};
// Do not accept a secret in a server-rendered input before React can handle
// its change event. Otherwise an early paste can be lost during hydration.
export function useHydrated() {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false,
  );
}
