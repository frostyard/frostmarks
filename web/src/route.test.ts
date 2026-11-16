// route.test.ts checks the app's routes, and the rule of a savable URL.

import { splitRoute } from "@frostyard/lodge-client";
import { describe, expect, it } from "vitest";
import { listRoute, parseRoute, savableURL } from "./route";

describe("parseRoute", () => {
  it("reads the root's list", () => {
    expect(parseRoute("/", "")).toEqual({ kind: "list", folder: "/", query: "" });
  });

  it("reads a folder's list", () => {
    expect(parseRoute("/", "?folder=/work")).toEqual({ kind: "list", folder: "/work", query: "" });
  });

  it("reads a search in a folder", () => {
    expect(parseRoute("/", "?folder=/work&q=go")).toEqual({ kind: "list", folder: "/work", query: "go" });
  });

  it("reads the save page, with its URL and title", () => {
    expect(parseRoute("/save", "?url=https%3A%2F%2Fexample.com%2Fa&title=An%20article")).toEqual({
      kind: "save",
      url: "https://example.com/a",
      title: "An article",
    });
  });
});

describe("listRoute", () => {
  it("gives the root's route", () => {
    expect(listRoute("/")).toBe("/");
  });

  it("round-trips through parseRoute", () => {
    const cases = [
      { folder: "/", query: "" },
      { folder: "/work", query: "" },
      { folder: "/work/reading", query: "go reading" },
    ];
    for (const one of cases) {
      const { pathname, search } = splitRoute(listRoute(one.folder, one.query));
      expect(parseRoute(pathname, search)).toEqual({ kind: "list", ...one });
    }
  });
});

describe("savableURL", () => {
  it("accepts http and https", () => {
    expect(savableURL("http://example.com/a")).toBe(true);
    expect(savableURL("https://example.com/a?b=c")).toBe(true);
  });

  it("refuses another scheme and a text that is no URL", () => {
    expect(savableURL("javascript:alert(1)")).toBe(false);
    expect(savableURL("ftp://x")).toBe(false);
    expect(savableURL("not a url")).toBe(false);
  });
});
