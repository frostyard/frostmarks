// App.test.tsx checks the list view: the folders, the search, and the sheets
// that add, change, move and delete a bookmark.

import { Code } from "@connectrpc/connect";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "./App";
import type { Bookmark } from "./api";
import { type FakeAPI, type FakeSeed, fakeAPI } from "./testing/fakeApi";
import { type FakeLodge, fakeLodge } from "./testing/fakeLodge";

const SEED: FakeSeed = {
  folders: ["/work", "/reading"],
  bookmarks: [
    { url: "https://example.com/a", title: "An article", tags: ["go", "reading"], folder: "/" },
    { url: "https://example.com/b", title: "Work doc", tags: ["go"], folder: "/work" },
  ],
};

/** renderApp draws the app at 1 route, with the seed's store. */
function renderApp(route = "/", api: FakeAPI = fakeAPI(SEED)): { api: FakeAPI; fake: FakeLodge } {
  const fake = fakeLodge(route);
  render(<App lodge={fake.lodge} api={api} />);
  return { api, fake };
}

/** idOf returns the id of the bookmark that holds 1 title. */
function idOf(api: FakeAPI, title: string): string {
  for (const [id, bookmark] of api.bookmarks) {
    if (bookmark.title === title) return id;
  }
  throw new Error(`no bookmark ${title}`);
}

/** Deferred is a promise that the test settles when it chooses. */
interface Deferred<T> {
  promise: Promise<T>;
  resolve(value: T): void;
  reject(reason: unknown): void;
}

/** deferred returns a promise and the 2 functions that settle it. */
function deferred<T>(): Deferred<T> {
  const settle: { resolve?: (value: T) => void; reject?: (reason: unknown) => void } = {};
  const promise = new Promise<T>((resolve, reject) => {
    settle.resolve = resolve;
    settle.reject = reject;
  });
  return {
    promise,
    resolve: (value) => settle.resolve?.(value),
    reject: (reason) => settle.reject?.(reason),
  };
}

/** narrowWindow makes the window answer as a phone does, and returns the undo. */
function narrowWindow(): () => void {
  const original = globalThis.matchMedia;
  globalThis.matchMedia = (query: string): MediaQueryList => ({
    media: query,
    matches: true,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => true,
  });
  return () => {
    globalThis.matchMedia = original;
  };
}

describe("App", () => {
  it("lists the bookmarks of the root", async () => {
    renderApp();
    const link = await screen.findByRole("link", { name: "An article" });
    expect(link.getAttribute("href")).toBe("https://example.com/a");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(screen.queryByRole("link", { name: "Work doc" })).toBeNull();
    expect(screen.getByRole("button", { name: "All bookmarks" }).getAttribute("aria-current")).toBe("page");
  });

  it("pushes the route of a folder and shows its bookmarks", async () => {
    const user = userEvent.setup();
    const { fake } = renderApp();
    await screen.findByRole("link", { name: "An article" });

    await user.click(screen.getByRole("button", { name: "/work" }));
    expect(fake.pushes).toContainEqual({ route: "/?folder=%2Fwork", title: "work" });
    expect(await screen.findByRole("link", { name: "Work doc" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "An article" })).toBeNull();
    expect(screen.getByRole("button", { name: "/work" }).getAttribute("aria-current")).toBe("page");
  });

  it("shows the matches of a search", async () => {
    const user = userEvent.setup();
    const { fake } = renderApp();
    await screen.findByRole("link", { name: "An article" });

    await user.type(screen.getByRole("searchbox", { name: "Search bookmarks" }), "article{Enter}");
    expect(fake.pushes).toContainEqual({ route: "/?q=article", title: "Search" });
    expect(await screen.findByRole("link", { name: "An article" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Work doc" })).toBeNull();
  });

  it("adds a bookmark in the sheet", async () => {
    const user = userEvent.setup();
    const { api } = renderApp();
    await screen.findByRole("link", { name: "An article" });

    await user.click(screen.getByRole("button", { name: "Add bookmark" }));
    const sheet = await screen.findByRole("dialog", { name: "Add bookmark" });
    await user.type(within(sheet).getByLabelText("URL"), "https://example.com/c");
    await user.type(within(sheet).getByLabelText("Title"), "New one");
    await user.type(within(sheet).getByLabelText("Tags"), "Go, Reading");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    expect(api.calls.createBookmark).toEqual([
      { url: "https://example.com/c", title: "New one", notes: "", tags: ["go", "reading"], folder: "/" },
    ]);
    expect(await screen.findByRole("link", { name: "New one" })).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("changes a bookmark's title and tags", async () => {
    const user = userEvent.setup();
    const { api } = renderApp();
    await screen.findByRole("link", { name: "An article" });
    const id = idOf(api, "An article");

    await user.click(screen.getByRole("button", { name: "Edit An article" }));
    const sheet = await screen.findByRole("dialog", { name: "Edit bookmark" });
    const title = within(sheet).getByLabelText("Title");
    await user.clear(title);
    await user.type(title, "A better article");
    const tags = within(sheet).getByLabelText("Tags");
    await user.clear(tags);
    await user.type(tags, "go golang");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    expect(api.calls.updateBookmark).toHaveLength(1);
    expect(api.calls.updateBookmark[0]?.id).toBe(id);
    expect(api.calls.updateBookmark[0]?.addTags).toEqual(["golang"]);
    expect(api.calls.updateBookmark[0]?.removeTags).toEqual(["reading"]);
    expect(api.calls.updateBookmark[0]?.input).toEqual({
      url: "https://example.com/a",
      title: "A better article",
      notes: "",
      tags: ["go", "golang"],
    });
    expect(api.calls.moveBookmark).toHaveLength(0);
    expect(await screen.findByRole("link", { name: "A better article" })).toBeTruthy();
  });

  it("moves a bookmark when its folder changes", async () => {
    const user = userEvent.setup();
    const { api } = renderApp();
    await screen.findByRole("link", { name: "An article" });
    const id = idOf(api, "An article");

    await user.click(screen.getByRole("button", { name: "Edit An article" }));
    const sheet = await screen.findByRole("dialog", { name: "Edit bookmark" });
    await user.selectOptions(within(sheet).getByLabelText("Folder"), "/reading");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    expect(api.calls.moveBookmark).toEqual([{ id, folder: "/reading" }]);
    expect(api.calls.updateBookmark).toHaveLength(1);
    expect(api.bookmarks.get(id)?.folder).toBe("/reading");
    expect(await screen.findByText("No bookmarks in this folder.")).toBeTruthy();
  });

  it("deletes a bookmark after the confirm", async () => {
    const user = userEvent.setup();
    const { api } = renderApp();
    await screen.findByRole("link", { name: "An article" });
    const id = idOf(api, "An article");

    await user.click(screen.getByRole("button", { name: "Delete An article" }));
    const sheet = await screen.findByRole("alertdialog", { name: "Delete bookmark" });
    expect(api.calls.deleteBookmark).toHaveLength(0);
    await user.click(within(sheet).getByRole("button", { name: "Delete" }));

    expect(api.calls.deleteBookmark).toEqual([id]);
    expect(api.bookmarks.has(id)).toBe(false);
    expect(await screen.findByText("No bookmarks in this folder.")).toBeTruthy();
  });

  it("shows a load's error, and reads again when Retry is chosen", async () => {
    const user = userEvent.setup();
    const api = fakeAPI(SEED);
    api.failNext("listBookmarks", Code.Unavailable, "the store is closed");
    renderApp("/", api);

    expect(await screen.findByText("the store is closed")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("link", { name: "An article" })).toBeTruthy();
  });

  it("makes a folder, and shows it in the tree", async () => {
    const user = userEvent.setup();
    const { api } = renderApp("/?folder=%2Fwork");
    await screen.findByRole("link", { name: "Work doc" });

    await user.click(screen.getByRole("button", { name: "New folder" }));
    const sheet = await screen.findByRole("dialog", { name: "New folder" });
    const path = within(sheet).getByLabelText("Folder path");
    expect((path as HTMLInputElement).value).toBe("/work/");
    await user.type(path, "later");
    await user.click(within(sheet).getByRole("button", { name: "Create" }));

    expect(api.calls.createFolder).toEqual(["/work/later"]);
    expect(await screen.findByRole("button", { name: "/work/later" })).toBeTruthy();
  });

  it("shows why a folder that is not empty is not deleted", async () => {
    const user = userEvent.setup();
    renderApp("/?folder=%2Fwork");
    await screen.findByRole("link", { name: "Work doc" });

    await user.click(screen.getByRole("button", { name: "Delete folder" }));
    const sheet = await screen.findByRole("alertdialog", { name: "Delete folder" });
    await user.click(within(sheet).getByRole("button", { name: "Delete" }));

    expect(await within(sheet).findByRole("alert")).toBeTruthy();
    expect(within(sheet).getByRole("alert").textContent).toBe("the folder /work is not empty");
    expect(screen.getByRole("alertdialog", { name: "Delete folder" })).toBeTruthy();
  });

  it("offers the bookmarklet as a link and as text", async () => {
    const user = userEvent.setup();
    renderApp();
    await screen.findByRole("link", { name: "An article" });

    await user.click(screen.getByRole("button", { name: "Bookmarklet" }));
    const sheet = await screen.findByRole("dialog", { name: "Save from your browser" });
    await screen.findByRole("link", { name: "Save to Bookmarks" });
    const link = within(sheet).getByRole("link", { name: "Save to Bookmarks" });
    expect(link.getAttribute("href")?.startsWith("javascript:")).toBe(true);
    const code = within(sheet).getByLabelText("Bookmarklet code") as HTMLTextAreaElement;
    expect(code.value).toBe(link.getAttribute("href"));
    expect(code.readOnly).toBe(true);
  });

  it("keeps the newer folder's rows when an old load answers late", async () => {
    const user = userEvent.setup();
    const api = fakeAPI({
      folders: ["/work", "/reading"],
      bookmarks: [
        { url: "https://example.com/b", title: "Work doc", folder: "/work" },
        { url: "https://example.com/c", title: "Reading doc", folder: "/reading" },
      ],
    });
    const real = api.listBookmarks.bind(api);
    const gates = new Map<string, Deferred<Bookmark[]>>();
    // Each list call waits until the test settles it.
    api.listBookmarks = async (folder) => {
      const gate = deferred<Bookmark[]>();
      gates.set(folder, gate);
      return gate.promise;
    };
    renderApp("/?folder=%2Fwork", api);

    await user.click(await screen.findByRole("button", { name: "/reading" }));
    await act(async () => {
      gates.get("/reading")?.resolve(await real("/reading", false));
    });
    expect(await screen.findByRole("link", { name: "Reading doc" })).toBeTruthy();

    // The load of /work answers late: its rows must not replace /reading's.
    await act(async () => {
      gates.get("/work")?.resolve(await real("/work", false));
    });
    expect(screen.getByRole("link", { name: "Reading doc" })).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Work doc" })).toBeNull();
  });

  it("keeps a saved change as the baseline when the move fails", async () => {
    const user = userEvent.setup();
    const api = fakeAPI({
      folders: ["/work", "/reading"],
      bookmarks: [{ url: "https://example.com/a", title: "An article", folder: "/" }],
    });
    renderApp("/", api);
    await screen.findByRole("link", { name: "An article" });
    const id = idOf(api, "An article");
    api.failNext("moveBookmark", Code.Unavailable, "the store is closed");

    await user.click(screen.getByRole("button", { name: "Edit An article" }));
    const sheet = await screen.findByRole("dialog", { name: "Edit bookmark" });
    await user.type(within(sheet).getByLabelText("Tags"), "reading");
    await user.selectOptions(within(sheet).getByLabelText("Folder"), "/reading");
    await user.click(within(sheet).getByRole("button", { name: "Save" }));

    // The update is stored, the move is not: the sheet stays, and says so.
    const alert = await within(sheet).findByRole("alert");
    expect(alert.textContent).toContain("The change is saved");
    expect(alert.textContent).toContain("the move to /reading failed");
    expect(api.bookmarks.get(id)?.tags).toEqual(["reading"]);
    expect(api.bookmarks.get(id)?.folder).toBe("/");

    // The next save removes that tag: it is computed from the saved change.
    await user.clear(within(sheet).getByLabelText("Tags"));
    await user.click(within(sheet).getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(api.bookmarks.get(id)?.tags).toEqual([]);
    expect(api.bookmarks.get(id)?.folder).toBe("/reading");
  });

  it("shows a folder error and a working Retry in a narrow window", async () => {
    const user = userEvent.setup();
    const undo = narrowWindow();
    try {
      const api = fakeAPI(SEED);
      api.failNext("listFolders", Code.Unavailable, "the folder list is closed");
      renderApp("/", api);

      expect(await screen.findByText("the folder list is closed")).toBeTruthy();
      await user.click(screen.getByRole("button", { name: "Retry" }));
      await waitFor(() => {
        expect(screen.queryByRole("alert")).toBeNull();
      });
      const select = screen.getByLabelText("Folder") as HTMLSelectElement;
      expect([...select.options].map((option) => option.value)).toContain("/work");
    } finally {
      undo();
    }
  });
});
