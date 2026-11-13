const guards = new Set<() => boolean>();
export function registerNavigationGuard(guard: () => boolean) {
  guards.add(guard);
  return () => {
    guards.delete(guard);
  };
}
export function confirmWorkspaceNavigation(): boolean {
  for (const guard of guards) if (!guard()) return false;
  return true;
}
