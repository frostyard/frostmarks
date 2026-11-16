// The bookmarklet (docs/specs/lodge-app.md, The bookmarklet): it opens the
// save page's canonical URL in a new window. lodge admits a top-level
// navigation to a shell page from any site, and refuses a cross-site request
// to an app route, so the URL is /marks/save and never /app/marks/save.

/**
 * bookmarklet returns the bookmarklet's javascript: URL for lodge's origin,
 * such as https://lodge.example.com, which holds no quote.
 */
export function bookmarklet(origin: string): string {
  return (
    `javascript:void(window.open('${origin}/marks/save?url='+encodeURIComponent(location.href)` +
    "+'&title='+encodeURIComponent(document.title),'_blank','noopener'))"
  );
}
