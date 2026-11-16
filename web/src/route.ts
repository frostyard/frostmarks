// The app's routes (docs/specs/lodge-app.md): the shell shows the canonical
// URL /marks<route>, and the frame is /app/marks<route>.
//
//   /                  the bookmarks of the root folder
//   /?folder=F         the bookmarks of folder F
//   /?folder=F&q=Q     the search of Q in folder F and below
//   /save?url=U&title=T  the save page

/** View is 1 parsed route. */
export type View = { kind: "list"; folder: string; query: string } | { kind: "save"; url: string; title: string };

/** parseRoute reads 1 route, such as lodge.history.location gives it. */
export function parseRoute(pathname: string, search: string): View {
  const params = new URLSearchParams(search);
  if (pathname === "/save") {
    return { kind: "save", url: params.get("url") ?? "", title: params.get("title") ?? "" };
  }
  return { kind: "list", folder: params.get("folder") || "/", query: params.get("q") ?? "" };
}

/** listRoute returns the route of a folder's list, and of a search in it. */
export function listRoute(folder: string, query = ""): string {
  const params = new URLSearchParams();
  if (folder !== "/") params.set("folder", folder);
  if (query !== "") params.set("q", query);
  const search = params.toString();
  return search === "" ? "/" : `/?${search}`;
}

/** savableURL reports whether a text is an http or https URL. */
export function savableURL(text: string): boolean {
  try {
    const url = new URL(text);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

/** LAST_FOLDER is the local-storage key, after lodge's "marks:" prefix, of the save page's last folder. */
export const LAST_FOLDER = "lastFolder";
