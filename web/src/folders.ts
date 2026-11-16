// folders.ts turns the API's folder list into what the UI shows: the tree's
// rows, a folder's name, and its parent.

import type { Folder } from "./api";

/** FolderRow is 1 folder as the tree shows it: its path, its name and its depth under the root. */
export interface FolderRow {
  path: string;
  name: string;
  depth: number;
}

/** folderName returns what a folder is called: "All bookmarks" for the root, else its last segment. */
export function folderName(path: string): string {
  if (path === "/") return "All bookmarks";
  return path.slice(path.lastIndexOf("/") + 1);
}

/** parentOf returns the folder that holds a path: "/" for a folder at the top. */
export function parentOf(path: string): string {
  const cut = path.lastIndexOf("/");
  return cut <= 0 ? "/" : path.slice(0, cut);
}

/** depthOf returns how many segments a folder path holds below the root. */
export function depthOf(path: string): number {
  return path.split("/").filter((segment) => segment !== "").length;
}

/** folderRows returns the root and every folder in tree order, so a parent comes before its children. */
export function folderRows(paths: string[]): FolderRow[] {
  const seen = new Set<string>();
  const rows: FolderRow[] = [{ path: "/", name: folderName("/"), depth: 0 }];
  for (const path of [...paths].sort()) {
    if (path === "/" || seen.has(path)) continue;
    seen.add(path);
    rows.push({ path, name: folderName(path), depth: depthOf(path) });
  }
  return rows;
}

/** folderPaths returns the folders in path order, without the root. */
export function folderPaths(folders: Folder[]): string[] {
  return folders
    .map((folder) => folder.path)
    .filter((path) => path !== "/")
    .sort();
}
