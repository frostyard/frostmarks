// App.test.tsx checks the list view: the folders, the search, and the sheets
// that add, change, move and delete a bookmark.

import { Code } from "@connectrpc/connect";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "./App";
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
});
