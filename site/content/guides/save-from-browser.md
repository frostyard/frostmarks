# Save from your browser

On lodge, a bookmarklet saves the page you read with 1 click: it opens
frostmarks' save page in a new window, with the page's address and title filled
in, and you pick the folder.

## Add the bookmarklet

Open frostmarks in lodge, open **Bookmarklet**, and drag the link **Save to
Bookmarks** to your browser's bookmarks bar. The link already holds your lodge's
address.

To make it by hand, add a bookmark whose address is this line, with `LODGE`
replaced by your lodge's name, such as `lodge.example.com`:

```text
javascript:void(window.open('https://LODGE/marks/save?url='+encodeURIComponent(location.href)+'&title='+encodeURIComponent(document.title),'_blank','noopener'))
```

## Save a page

1. Open the page to keep, and click **Save to Bookmarks**.
2. A new window opens lodge with the save page. Check the address and the title,
   and add notes and tags if you like.
3. Pick a folder, or make a new 1, and press **Save**.

The window remembers the folder of your last save. A page that is already in
that folder is not saved again: the page says which bookmark holds it.

## Why the address is `/marks/save`

The bookmarklet opens lodge's own page for the app, `/marks/save`, which lodge
opens from any site, and lodge then loads the save page inside it. lodge refuses
a request from another site to the app's frame, `/app/marks/save`, directly.
Only `http` and `https` pages can be saved.

## Next

[Run on lodge](lodge.md) covers the install.
