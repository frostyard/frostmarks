// App.tsx is the web UI: it shows the list view or the save page, from the
// route that lodge's shell and the app keep.

import { type Lodge, splitRoute } from "@frostyard/lodge-client";
import { useSyncExternalStore } from "react";
import type { MarksAPI } from "./api";
import { ListView } from "./ListView";
import { parseRoute } from "./route";
import { SavePage } from "./SavePage";

/** useRoute follows lodge's history: the app's push and the shell's navigate. */
function useRoute(lodge: Lodge): string {
  return useSyncExternalStore(
    (onChange) => lodge.history.listen(() => onChange()),
    () => lodge.history.location.pathname + lodge.history.location.search,
  );
}

/** App is the app's page: the bookmarks of a folder, or the save page of 1 URL. */
export function App({ lodge, api }: { lodge: Lodge; api: MarksAPI }) {
  const route = useRoute(lodge);
  const { pathname, search } = splitRoute(route);
  const view = parseRoute(pathname, search);
  if (view.kind === "save") {
    return <SavePage key={route} lodge={lodge} api={api} url={view.url} title={view.title} />;
  }
  return <ListView lodge={lodge} api={api} folder={view.folder} query={view.query} />;
}
