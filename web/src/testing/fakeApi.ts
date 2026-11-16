// fakeApi.ts is an in-memory MarksAPI for the tests. It keeps the rules of
// the API that the UI meets, and it records every call.

import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import type { Bookmark, BookmarkInput, MarksAPI } from "../api";
import { BookmarkSchema, FolderSchema } from "../gen/frostyard/frostmarks/v1/marks_pb";

/** FakeSeed is the store that a fake starts with. */
export interface FakeSeed {
  /** Every folder except the root, which always exists. */
  folders?: string[];
  bookmarks?: Array<Pick<BookmarkInput, "url"> & Partial<Omit<BookmarkInput, "url">>>;
}

/** FakeCalls is what the UI called, in order. */
export interface FakeCalls {
  listFolders: number;
  listBookmarks: Array<{ folder: string; recursive: boolean }>;
  searchBookmarks: Array<{ query: string; folder: string }>;
  createBookmark: BookmarkInput[];
  updateBookmark: Array<{
    id: string;
    input: Omit<BookmarkInput, "folder">;
    addTags: string[];
    removeTags: string[];
  }>;
  moveBookmark: Array<{ id: string; folder: string }>;
  deleteBookmark: string[];
  createFolder: string[];
  renameFolder: Array<{ path: string; newPath: string }>;
  deleteFolder: string[];
}

/** FakeAPI is the fake, with its store and the calls it recorded. */
export interface FakeAPI extends MarksAPI {
  readonly folders: Set<string>;
  readonly bookmarks: Map<string, Bookmark>;
  readonly calls: FakeCalls;
  /** failNext makes the next call of 1 method fail with a Connect error. */
  failNext(method: keyof MarksAPI, code?: Code, message?: string): void;
}

/** normalizeURL is the API's URL: trimmed, with a lower-case scheme and host, and no fragment. */
function normalizeURL(text: string): string {
  try {
    const url = new URL(text.trim());
    url.hash = "";
    return url.toString();
  } catch {
    return text.trim();
  }
}

/** lowerTags is the API's tag list: trimmed, lower-case, sorted, without a repeat. */
function lowerTags(tags: string[]): string[] {
  return [...new Set(tags.map((tag) => tag.trim().toLowerCase()).filter((tag) => tag !== ""))].sort();
}

/** fakeAPI returns a MarksAPI whose store lives in memory. */
export function fakeAPI(seed: FakeSeed = {}): FakeAPI {
  const folders = new Set<string>(seed.folders ?? []);
  const bookmarks = new Map<string, Bookmark>();
  const failures = new Map<keyof MarksAPI, ConnectError>();
  const calls: FakeCalls = {
    listFolders: 0,
    listBookmarks: [],
    searchBookmarks: [],
    createBookmark: [],
    updateBookmark: [],
    moveBookmark: [],
    deleteBookmark: [],
    createFolder: [],
    renameFolder: [],
    deleteFolder: [],
  };
  let made = 0;

  /** guard throws the failure that failNext set for 1 method. */
  const guard = (method: keyof MarksAPI): void => {
    const err = failures.get(method);
    if (err === undefined) return;
    failures.delete(method);
    throw err;
  };

  /** hasFolder reports whether a path names a folder that exists. */
  const hasFolder = (path: string): boolean => path === "/" || folders.has(path);

  /** needFolder throws when a folder does not exist. */
  const needFolder = (path: string): void => {
    if (!hasFolder(path)) throw new ConnectError(`no folder ${path}`, Code.NotFound);
  };

  /** needBookmark returns a bookmark, or throws when it is not there. */
  const needBookmark = (id: string): Bookmark => {
    const found = bookmarks.get(id);
    if (found === undefined) throw new ConnectError(`no bookmark ${id}`, Code.NotFound);
    return found;
  };

  /** inFolder reports whether a bookmark's folder is 1 path or a folder below it. */
  const inFolder = (folder: string, path: string): boolean =>
    folder === "/" || path === folder || path.startsWith(`${folder}/`);

  /** sameURL is the bookmark of that URL in that folder, if there is 1. */
  const sameURL = (url: string, folder: string, except?: string): Bookmark | undefined => {
    for (const bookmark of bookmarks.values()) {
      if (bookmark.id !== except && bookmark.folder === folder && bookmark.url === url) return bookmark;
    }
    return undefined;
  };

  /** put stores a bookmark under a new id. */
  const put = (input: BookmarkInput): Bookmark => {
    made += 1;
    const bookmark = create(BookmarkSchema, {
      id: `bookmark-${made}`,
      url: normalizeURL(input.url),
      title: input.title,
      notes: input.notes,
      tags: lowerTags(input.tags),
      folder: input.folder === "" ? "/" : input.folder,
    });
    bookmarks.set(bookmark.id, bookmark);
    return bookmark;
  };

  for (const entry of seed.bookmarks ?? []) {
    put({
      url: entry.url,
      title: entry.title ?? "",
      notes: entry.notes ?? "",
      tags: entry.tags ?? [],
      folder: entry.folder ?? "/",
    });
  }

  return {
    folders,
    bookmarks,
    calls,
    failNext(method, code = Code.Unavailable, message = "the app failed") {
      failures.set(method, new ConnectError(message, code));
    },
    async listFolders() {
      guard("listFolders");
      calls.listFolders += 1;
      return [...folders].sort().map((path) => create(FolderSchema, { path }));
    },
    async listBookmarks(folder, recursive) {
      guard("listBookmarks");
      calls.listBookmarks.push({ folder, recursive });
      needFolder(folder);
      return [...bookmarks.values()]
        .filter((bookmark) => (recursive ? inFolder(folder, bookmark.folder) : bookmark.folder === folder))
        .sort((a, b) => (a.folder === b.folder ? a.id.localeCompare(b.id) : a.folder.localeCompare(b.folder)));
    },
    async searchBookmarks(query, folder) {
      guard("searchBookmarks");
      calls.searchBookmarks.push({ query, folder });
      needFolder(folder);
      const words = query
        .toLowerCase()
        .split(/[\s,]+/)
        .filter((word) => word !== "");
      if (words.length === 0) throw new ConnectError("the query holds no word", Code.InvalidArgument);
      return [...bookmarks.values()].filter((bookmark) => {
        if (!inFolder(folder, bookmark.folder)) return false;
        const hay = [bookmark.title, bookmark.url, bookmark.notes, ...bookmark.tags].join(" ").toLowerCase();
        return words.every((word) => hay.includes(word));
      });
    },
    async createBookmark(input) {
      guard("createBookmark");
      calls.createBookmark.push(input);
      const folder = input.folder === "" ? "/" : input.folder;
      needFolder(folder);
      const url = normalizeURL(input.url);
      const existing = sameURL(url, folder);
      if (existing !== undefined) {
        throw new ConnectError(
          `the URL ${url} is already bookmarked in ${folder} as ${existing.id}`,
          Code.AlreadyExists,
        );
      }
      return put({ ...input, folder });
    },
    async updateBookmark(id, input, addTags, removeTags) {
      guard("updateBookmark");
      calls.updateBookmark.push({ id, input, addTags, removeTags });
      const old = needBookmark(id);
      const url = normalizeURL(input.url);
      const existing = sameURL(url, old.folder, id);
      if (existing !== undefined) {
        throw new ConnectError(
          `the URL ${url} is already bookmarked in ${old.folder} as ${existing.id}`,
          Code.AlreadyExists,
        );
      }
      const kept = old.tags.filter((tag) => !lowerTags(removeTags).includes(tag));
      const next = create(BookmarkSchema, {
        ...old,
        url,
        title: input.title,
        notes: input.notes,
        tags: lowerTags([...kept, ...addTags]),
      });
      bookmarks.set(id, next);
      return next;
    },
    async moveBookmark(id, folder) {
      guard("moveBookmark");
      calls.moveBookmark.push({ id, folder });
      const old = needBookmark(id);
      needFolder(folder);
      if (folder !== old.folder) {
        const existing = sameURL(old.url, folder, id);
        if (existing !== undefined) {
          throw new ConnectError(
            `the URL ${old.url} is already bookmarked in ${folder} as ${existing.id}`,
            Code.AlreadyExists,
          );
        }
      }
      const next = create(BookmarkSchema, { ...old, folder });
      bookmarks.set(id, next);
      return next;
    },
    async deleteBookmark(id) {
      guard("deleteBookmark");
      calls.deleteBookmark.push(id);
      needBookmark(id);
      bookmarks.delete(id);
    },
    async createFolder(path) {
      guard("createFolder");
      calls.createFolder.push(path);
      if (path === "/") throw new ConnectError("the root folder / always exists", Code.InvalidArgument);
      if (folders.has(path)) throw new ConnectError(`the folder ${path} exists`, Code.AlreadyExists);
      const cut = path.lastIndexOf("/");
      needFolder(cut <= 0 ? "/" : path.slice(0, cut));
      folders.add(path);
      return create(FolderSchema, { path });
    },
    async renameFolder(path, newPath) {
      guard("renameFolder");
      calls.renameFolder.push({ path, newPath });
      needFolder(path);
      if (folders.has(newPath)) throw new ConnectError(`the folder ${newPath} exists`, Code.AlreadyExists);
      for (const folder of [...folders]) {
        if (folder === path || folder.startsWith(`${path}/`)) {
          folders.delete(folder);
          folders.add(`${newPath}${folder.slice(path.length)}`);
        }
      }
      for (const [id, bookmark] of bookmarks) {
        if (bookmark.folder === path || bookmark.folder.startsWith(`${path}/`)) {
          bookmarks.set(
            id,
            create(BookmarkSchema, { ...bookmark, folder: `${newPath}${bookmark.folder.slice(path.length)}` }),
          );
        }
      }
      return create(FolderSchema, { path: newPath });
    },
    async deleteFolder(path) {
      guard("deleteFolder");
      calls.deleteFolder.push(path);
      needFolder(path);
      const holdsBookmark = [...bookmarks.values()].some((bookmark) => bookmark.folder === path);
      const holdsFolder = [...folders].some((folder) => folder !== path && folder.startsWith(`${path}/`));
      if (holdsBookmark || holdsFolder) {
        throw new ConnectError(`the folder ${path} is not empty`, Code.FailedPrecondition);
      }
      folders.delete(path);
    },
  };
}
