// SavePage.test.tsx is the acceptance test of the save page: the fields that
// the route fills, the folder of the save, the last folder, and the errors.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "./App";
import { type FakeAPI, fakeAPI } from "./testing/fakeApi";
import { fakeLodge } from "./testing/fakeLodge";

const SAVE = "/save?url=https%3A%2F%2Fexample.com%2Fa&title=An%20article";

/** renderSave draws the app at 1 save route, with a folder list and 1 fake API. */
function renderSave(route = SAVE, api: FakeAPI = fakeAPI({ folders: ["/work", "/reading"] })) {
  const fake = fakeLodge(route);
  const view = render(<App lodge={fake.lodge} api={api} />);
  return { api, fake, view };
}

/** folderSelect returns the folder select, once the folders are read. */
async function folderSelect(): Promise<HTMLSelectElement> {
  return (await screen.findByLabelText("Folder")) as HTMLSelectElement;
}

describe("SavePage", () => {
  it("fills the fields from the route, and lists the folders", async () => {
    renderSave();
    expect(((await screen.findByLabelText("URL")) as HTMLInputElement).value).toBe("https://example.com/a");
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe("An article");
    const select = await folderSelect();
    expect([...select.options].map((option) => option.value)).toEqual(["/", "/reading", "/work"]);
  });

  it("saves into the folder that is chosen, and keeps that folder", async () => {
    const user = userEvent.setup();
    const { api, view } = renderSave();
    await user.selectOptions(await folderSelect(), "/reading");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Saved to /reading.")).toBeTruthy();
    expect(api.calls.createBookmark).toEqual([
      { url: "https://example.com/a", title: "An article", notes: "", tags: [], folder: "/reading" },
    ]);
    expect(api.bookmarks.size).toBe(1);
    expect(localStorage.getItem("marks:lastFolder")).toBe("/reading");

    // The next render of the page chooses the folder of the last save.
    view.unmount();
    const next = fakeLodge(SAVE);
    render(<App lodge={next.lodge} api={api} />);
    expect((await folderSelect()).value).toBe("/reading");
  });

  it("shows the API's text when the URL is already saved in the folder", async () => {
    const user = userEvent.setup();
    const { api } = renderSave();
    await user.selectOptions(await folderSelect(), "/reading");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByText("Saved to /reading.");

    await user.click(screen.getByRole("button", { name: "Save" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("already bookmarked in /reading as");
    expect(api.bookmarks.size).toBe(1);
  });

  it("refuses a URL that is neither http nor https", async () => {
    renderSave("/save?url=javascript%3Aalert(1)");
    expect(await screen.findByText("This page cannot be saved: only http and https addresses can be.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("makes a folder, and chooses it", async () => {
    const user = userEvent.setup();
    const { api } = renderSave();
    await folderSelect();
    await user.click(screen.getByRole("button", { name: "New folder…" }));
    await user.type(screen.getByLabelText("New folder path"), "/reading/later");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(api.calls.createFolder).toEqual(["/reading/later"]);
    expect((await folderSelect()).value).toBe("/reading/later");
  });
});
