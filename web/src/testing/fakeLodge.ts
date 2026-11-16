// fakeLodge.ts is a Lodge for the tests: its history lives in memory, and the
// shell's other members are the few that the UI reads.

import {
  type HistoryUpdate,
  type Lodge,
  type LodgeHistory,
  type LodgeLocation,
  splitRoute,
} from "@frostyard/lodge-client";

/** FakeLodge is the fake: the Lodge to give the app, and what it was asked to show. */
export interface FakeLodge {
  lodge: Lodge;
  /** Every route that the app pushed, with its title. */
  pushes: Array<{ route: string; title: string }>;
  /** Every route that the app replaced, with its title. */
  replaces: Array<{ route: string; title: string }>;
  /** navigate goes where the shell's own navigate message would. */
  navigate(route: string): void;
}

/** fakeLodge returns a Lodge whose history starts at 1 route. */
export function fakeLodge(route = "/"): FakeLodge {
  let location: LodgeLocation = splitRoute(route);
  const listeners = new Set<(update: HistoryUpdate) => void>();
  const pushes: Array<{ route: string; title: string }> = [];
  const replaces: Array<{ route: string; title: string }> = [];

  /** go moves the history and calls its listeners, as lodge does. */
  const go = (action: "push" | "replace", next: string, title = ""): void => {
    location = splitRoute(next);
    if (action === "push") pushes.push({ route: next, title });
    else replaces.push({ route: next, title });
    for (const listener of [...listeners]) listener({ action, location });
  };

  const history: LodgeHistory = {
    get location() {
      return location;
    },
    push: (next, title) => go("push", next, title),
    replace: (next, title) => go("replace", next, title),
    listen: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };

  // The UI reads only these members: the rest is the shape of a Lodge.
  const lodge = {
    app: "marks",
    contract: 1,
    theme: "light",
    shortcut: "Ctrl+Backquote",
    role: "app",
    companion: null,
    contentOrigin: null,
    visible: true,
    history,
    apiURL: () => "ws://localhost/app/marks/api",
    contentURL: (path: string) => `https://content.example.com/marks/${path}`,
    route: (path: string, title = "", options: { replace?: boolean } = {}) =>
      go(options.replace === true ? "replace" : "push", path, title),
    context: () => undefined,
    ask: () => undefined,
    open: () => undefined,
    openSwitcher: () => undefined,
    storageKey: (key: string) => `marks:${key}`,
    on: () => () => undefined,
    close: () => undefined,
  } as unknown as Lodge;

  return {
    lodge,
    pushes,
    replaces,
    navigate(next) {
      location = splitRoute(next);
      for (const listener of [...listeners]) listener({ action: "navigate", location });
    },
  };
}
