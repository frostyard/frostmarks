// bookmarklet.test.ts checks what the bookmarklet's code opens.

import { describe, expect, it } from "vitest";
import { bookmarklet } from "./bookmarklet";

const code = bookmarklet("https://lodge.example.com");

describe("bookmarklet", () => {
  it("is a javascript: URL", () => {
    expect(code.startsWith("javascript:")).toBe(true);
  });

  it("opens the save page's canonical URL", () => {
    expect(code).toContain("https://lodge.example.com/marks/save?url=");
  });

  it("never opens the frame's route", () => {
    expect(code).not.toContain("/app/marks/save");
  });

  it("opens the window without an opener", () => {
    expect(code).toContain("noopener");
  });
});
