// ListView.tsx is the app's main view: the folders, the search field and the
// rows of 1 folder, with the sheets that add, change and delete them.

import type { Lodge } from "@frostyard/lodge-client";
import { IconButton, useNarrow } from "@frostyard/lodge-ui/react";
import { Pencil, Plus, Trash2, X } from "lucide-react";
import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { type Bookmark, errorText, type Folder, type MarksAPI } from "./api";
import { BookmarkletSheet } from "./BookmarkletSheet";
import { BookmarkSheet } from "./BookmarkSheet";
import { ConfirmSheet } from "./ConfirmSheet";
import { ErrorLine } from "./ErrorLine";
import { FolderPanel } from "./FolderPanel";
import { folderName, parentOf } from "./folders";
import { PromptSheet } from "./PromptSheet";
import { listRoute } from "./route";
import { BUTTON, INPUT, QUIET } from "./ui";
import { hostOf } from "./url";

/** LoadState is what 1 load is doing: waiting, failed, or ready with its value. */
type LoadState<T> = { status: "loading" } | { status: "error"; text: string } | { status: "ready"; value: T };

/** SheetState is the sheet that the view shows, if any. */
type SheetState =
  | { kind: "add" }
  | { kind: "edit"; bookmark: Bookmark }
  | { kind: "deleteBookmark"; bookmark: Bookmark }
  | { kind: "newFolder" }
  | { kind: "renameFolder" }
  | { kind: "deleteFolder" }
  | { kind: "bookmarklet" };

/** ListViewProps is what the list view takes: the route's folder and query. */
export interface ListViewProps {
  lodge: Lodge;
  api: MarksAPI;
  folder: string;
  query: string;
}

/** ListView is the bookmarks of 1 folder, or the search of a query in it. */
export function ListView({ lodge, api, folder, query }: ListViewProps) {
  const narrow = useNarrow();
  const [folderState, setFolderState] = useState<LoadState<Folder[]>>({ status: "loading" });
  const [bookmarkState, setBookmarkState] = useState<LoadState<Bookmark[]>>({ status: "loading" });
  const [sheet, setSheet] = useState<SheetState | null>(null);
  const [draft, setDraft] = useState(query);

  const route = listRoute(folder, query);
  const title = query === "" ? folderName(folder) : "Search";

  useEffect(() => {
    setDraft(query);
  }, [query]);

  // The shell names the window after the view: on the first load, and when
  // the title changes.
  const told = useRef<string | null>(null);
  useEffect(() => {
    if (told.current === title) return;
    told.current = title;
    lodge.history.replace(route, title);
  }, [lodge, route, title]);

  // The folders, and then again after each change.
  const loadFolders = useCallback(async () => {
    try {
      const list = await api.listFolders();
      setFolderState({ status: "ready", value: list });
    } catch (err) {
      setFolderState({ status: "error", text: errorText(err) });
    }
  }, [api]);

  // The bookmarks of the folder, or the matches of the query.
  const loadBookmarks = useCallback(async () => {
    try {
      const list = query === "" ? await api.listBookmarks(folder, false) : await api.searchBookmarks(query, folder);
      setBookmarkState({ status: "ready", value: list });
    } catch (err) {
      setBookmarkState({ status: "error", text: errorText(err) });
    }
  }, [api, folder, query]);

  useEffect(() => {
    void loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    setBookmarkState({ status: "loading" });
    void loadBookmarks();
  }, [loadBookmarks]);

  /** reload reads the bookmarks and the folders again. */
  const reload = () => {
    void loadFolders();
    void loadBookmarks();
  };

  /** closeSheet takes the sheet away. */
  const closeSheet = () => setSheet(null);

  /** selectFolder shows another folder's bookmarks. */
  const selectFolder = (path: string) => {
    lodge.history.push(listRoute(path), folderName(path));
  };

  /** submitSearch shows the search of the field's text in the current folder. */
  const submitSearch = (event: FormEvent) => {
    event.preventDefault();
    const next = draft.trim();
    lodge.history.push(listRoute(folder, next), next === "" ? folderName(folder) : "Search");
  };

  /** clearSearch shows the folder's bookmarks again. */
  const clearSearch = () => {
    setDraft("");
    lodge.history.push(listRoute(folder), folderName(folder));
  };

  const folders: Folder[] = folderState.status === "ready" ? folderState.value : [];
  const bookmarks: Bookmark[] = bookmarkState.status === "ready" ? bookmarkState.value : [];

  return (
    <div className="flex h-full min-h-0 flex-row bg-window font-sans text-primary narrow:flex-col">
      <FolderPanel
        folders={folders}
        error={folderState.status === "error" ? folderState.text : null}
        current={folder}
        narrow={narrow}
        onSelect={selectFolder}
        onRetry={reload}
        onNew={() => setSheet({ kind: "newFolder" })}
        onRename={() => setSheet({ kind: "renameFolder" })}
        onDelete={() => setSheet({ kind: "deleteFolder" })}
        onBookmarklet={() => setSheet({ kind: "bookmarklet" })}
      />
      <main className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-1 border-separator border-b bg-toolbar px-2 py-1">
          <form className="flex min-w-0 flex-1 items-center gap-1" onSubmit={submitSearch}>
            <input
              type="search"
              aria-label="Search bookmarks"
              placeholder="Search bookmarks"
              className={INPUT}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
            />
            <button type="submit" className={BUTTON}>
              Search
            </button>
            {draft === "" ? null : <IconButton label="Clear search" icon={X} size={24} onClick={clearSearch} />}
          </form>
          <IconButton label="Add bookmark" icon={Plus} onClick={() => setSheet({ kind: "add" })} />
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">
          {bookmarkState.status === "loading" ? (
            <p className="px-3 py-2 text-secondary">Loading…</p>
          ) : bookmarkState.status === "error" ? (
            <ErrorLine text={bookmarkState.text} onRetry={reload} />
          ) : bookmarks.length === 0 ? (
            <p className="px-3 py-2 text-secondary">
              {query === "" ? "No bookmarks in this folder." : "No bookmark matches."}
            </p>
          ) : (
            <ul className="divide-y divide-separator">
              {bookmarks.map((bookmark) => (
                <BookmarkRow
                  key={bookmark.id}
                  bookmark={bookmark}
                  showFolder={query !== ""}
                  onEdit={() => setSheet({ kind: "edit", bookmark })}
                  onDelete={() => setSheet({ kind: "deleteBookmark", bookmark })}
                />
              ))}
            </ul>
          )}
        </div>
      </main>
      {sheet?.kind === "add" ? (
        <BookmarkSheet
          api={api}
          folders={folders}
          initial={null}
          folder={folder}
          onClose={closeSheet}
          onSaved={() => {
            closeSheet();
            reload();
          }}
        />
      ) : null}
      {sheet?.kind === "edit" ? (
        <BookmarkSheet
          api={api}
          folders={folders}
          initial={sheet.bookmark}
          folder={folder}
          onClose={closeSheet}
          onSaved={() => {
            closeSheet();
            reload();
          }}
        />
      ) : null}
      {sheet?.kind === "deleteBookmark" ? (
        <ConfirmSheet
          title="Delete bookmark"
          description={`Delete "${sheet.bookmark.title === "" ? sheet.bookmark.url : sheet.bookmark.title}"? This cannot be undone.`}
          confirmLabel="Delete"
          onConfirm={async () => {
            await api.deleteBookmark(sheet.bookmark.id);
            reload();
          }}
          onClose={closeSheet}
        />
      ) : null}
      {sheet?.kind === "newFolder" ? (
        <PromptSheet
          title="New folder"
          label="Folder path"
          initial={folder === "/" ? "/" : `${folder}/`}
          submit="Create"
          onSubmit={async (path) => {
            await api.createFolder(path);
            reload();
            selectFolder(path);
          }}
          onClose={closeSheet}
        />
      ) : null}
      {sheet?.kind === "renameFolder" ? (
        <PromptSheet
          title="Rename folder"
          label="New path"
          initial={folder}
          submit="Rename"
          onSubmit={async (path) => {
            await api.renameFolder(folder, path);
            reload();
            selectFolder(path);
          }}
          onClose={closeSheet}
        />
      ) : null}
      {sheet?.kind === "deleteFolder" ? (
        <ConfirmSheet
          title="Delete folder"
          description={`Delete the folder ${folder}? A folder that holds a bookmark or a folder is refused.`}
          confirmLabel="Delete"
          onConfirm={async () => {
            await api.deleteFolder(folder);
            reload();
            selectFolder(parentOf(folder));
          }}
          onClose={closeSheet}
        />
      ) : null}
      {sheet?.kind === "bookmarklet" ? <BookmarkletSheet onClose={closeSheet} /> : null}
    </div>
  );
}

/** BookmarkRow is 1 bookmark: its link, its host, its tags, and its buttons. */
function BookmarkRow({
  bookmark,
  showFolder,
  onEdit,
  onDelete,
}: {
  bookmark: Bookmark;
  /** A search shows the folder of each match. */
  showFolder: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const name = bookmark.title === "" ? bookmark.url : bookmark.title;
  const host = hostOf(bookmark.url);
  return (
    <li className="flex items-start gap-2 px-2 py-1.5">
      <div className="min-w-0 flex-1">
        <a
          href={bookmark.url}
          target="_blank"
          rel="noopener noreferrer"
          className="block truncate text-primary hover:underline"
        >
          {name}
        </a>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sidebar-row text-secondary">
          {host === "" ? null : <span className="truncate">{host}</span>}
          {showFolder ? <span className="text-tertiary">{bookmark.folder}</span> : null}
          {bookmark.tags.length === 0 ? null : (
            <ul aria-label="Tags" className="flex flex-wrap gap-1">
              {bookmark.tags.map((tag) => (
                <li key={tag} className="rounded bg-badge px-1.5 text-badge-text text-sidebar-section">
                  {tag}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <button type="button" className={QUIET} aria-label={`Edit ${name}`} onClick={onEdit}>
          <Pencil size={14} aria-hidden="true" />
        </button>
        <button type="button" className={QUIET} aria-label={`Delete ${name}`} onClick={onDelete}>
          <Trash2 size={14} aria-hidden="true" />
        </button>
      </div>
    </li>
  );
}
