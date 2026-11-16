// url.ts reads the parts of a bookmark's URL that the list shows.

/** hostOf returns the host of a URL, such as example.com, and "" when the URL does not parse. */
export function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return "";
  }
}
