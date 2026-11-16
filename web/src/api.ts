// The app's API in the browser: a Connect client of frostyard.frostmarks.v1.Marks,
// generated from the proto (src/gen, make web-generate), over lodge's app
// calls route, /app/marks/connect/ (docs/specs/lodge-app.md). lodge checks the
// browser's Origin and session, and the daemon checks the signed-in person's
// grant, which holds every capability on the root folder.
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { type Bookmark, type Folder, Marks } from "./gen/frostyard/frostmarks/v1/marks_pb";

export type { Bookmark, Folder };

/** BASE is the path under which lodge proxies the app's Connect calls. */
export const BASE = "/app/marks/connect";

/** BookmarkInput is what a form sends to create or change 1 bookmark. */
export interface BookmarkInput {
  url: string;
  title: string;
  notes: string;
  tags: string[];
  folder: string;
}

/**
 * MarksAPI is what the UI calls: 1 method for each RPC that it uses. The
 * tests give a fake of it, so no test needs a server.
 */
export interface MarksAPI {
  listFolders(): Promise<Folder[]>;
  listBookmarks(folder: string, recursive: boolean): Promise<Bookmark[]>;
  searchBookmarks(query: string, folder: string): Promise<Bookmark[]>;
  createBookmark(input: BookmarkInput): Promise<Bookmark>;
  /** updateBookmark sets the URL, title and notes, and adds and removes tags. */
  updateBookmark(
    id: string,
    input: Omit<BookmarkInput, "folder">,
    addTags: string[],
    removeTags: string[],
  ): Promise<Bookmark>;
  moveBookmark(id: string, folder: string): Promise<Bookmark>;
  deleteBookmark(id: string): Promise<void>;
  createFolder(path: string): Promise<Folder>;
  renameFolder(path: string, newPath: string): Promise<Folder>;
  deleteFolder(path: string): Promise<void>;
}

/** missing is the error of a response that holds no message where 1 must be. */
function missing(what: string): Error {
  return new Error(`the response holds no ${what}`);
}

/** connectAPI returns the API over lodge's app calls route. */
export function connectAPI(baseUrl: string = BASE): MarksAPI {
  const client = createClient(Marks, createConnectTransport({ baseUrl }));
  const one = <T>(value: T | undefined, what: string): T => {
    if (value === undefined) throw missing(what);
    return value;
  };
  return {
    async listFolders() {
      return (await client.listFolders({ folder: "/", recursive: true })).folders;
    },
    async listBookmarks(folder, recursive) {
      return (await client.listBookmarks({ folder, recursive })).bookmarks;
    },
    async searchBookmarks(query, folder) {
      return (await client.searchBookmarks({ query, folder })).bookmarks;
    },
    async createBookmark(input) {
      return one((await client.createBookmark(input)).bookmark, "bookmark");
    },
    async updateBookmark(id, input, addTags, removeTags) {
      const { url, title, notes } = input;
      return one((await client.updateBookmark({ id, url, title, notes, addTags, removeTags })).bookmark, "bookmark");
    },
    async moveBookmark(id, folder) {
      return one((await client.moveBookmark({ id, folder })).bookmark, "bookmark");
    },
    async deleteBookmark(id) {
      await client.deleteBookmark({ id });
    },
    async createFolder(path) {
      return one((await client.createFolder({ path })).folder, "folder");
    },
    async renameFolder(path, newPath) {
      return one((await client.renameFolder({ path, newPath })).folder, "folder");
    },
    async deleteFolder(path) {
      await client.deleteFolder({ path });
    },
  };
}

/** errorText is 1 line that says why a call failed, as the API words it. */
export function errorText(err: unknown): string {
  if (err instanceof ConnectError) return err.rawMessage;
  if (err instanceof Error) return err.message;
  return String(err);
}

/** alreadyExists reports whether a call failed because the URL is in the folder. */
export function alreadyExists(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.AlreadyExists;
}
