# Chat rich messages

Chat requires a Google Workspace account.

## Read full messages

```bash
gog --account you@example.com chat messages list spaces/AAA --json --full
gog --account you@example.com chat messages search 'release' --json --full --view full --markup markdown
gog --account you@example.com chat messages get spaces/AAA/messages/BBB --json
gog --account you@example.com chat messages get BBB --space AAA --plain
```

List and search keep their compact JSON and TSV defaults. `--full` is an output
mode and requires JSON; invalid combinations fail before credential/service
lookup. `--view full` is a separate **search provider option** for read/mute
metadata, subject to Google's available permissions. `--full` does not enable
that provider view automatically. `--markup chat|markdown` selects the provider's
formatted-text syntax on list, search, and get; omitting it leaves Google's
default unchanged. The syntax changes `formattedText`, which list/get expose
in full JSON; it does not change their plain-text summaries.

Full list returns `{ "messages": [...], "nextPageToken": "..." }`, using Google's
`name` field instead of the compact `resource` alias. Full search keeps its
`results` envelope and paging aggregates; each result contains a full `message`
and available `read`/`spaceMuteSetting` metadata. Absent read state stays absent,
while explicit `false` stays false. Get and update return `{ "message": {...} }`.
Get/update text and plain output use the concise message summary columns.

Full output serializes the current Google SDK `Message` type. It includes
SDK-known formatted text, annotations (including rich links), attachments, cards
V1/V2, GIFs, accessory widgets, emoji image references, and other message
metadata when returned by Google. Card-only messages are retained. This is
**typed output, not raw or lossless JSON**: unknown server fields and omitted
zero/empty fields are not preserved by the SDK. The feature does not render
cards, fetch image URLs, or download image/attachment bytes. URLs can expire or
require authorization. Full output makes no additional content-fetch requests.

Paging options `--page`, `--all`, and `--fail-empty` retain their existing
behavior; exit code 3 reports empty results when requested. Full mode does not
change the server-side search, unread, thread, or ordering filters.

## Consume content safely

```bash
gog --readonly --account you@example.com chat messages list spaces/AAA \
  --full --json --wrap-untrusted --results-only \
  --select name,text,cardsV2,attachment
gog --readonly --account you@example.com chat messages get spaces/AAA/messages/BBB \
  --json --wrap-untrusted --results-only \
  --select name,cardsV2.0.card.header.title,cardsV2.0.card.header.imageUrl
```

`--wrap-untrusted` treats rich message strings conservatively, including card
text, titles/subtitles, labels, alt text, filenames, and button/action content.
Validated server-assigned resource names, opaque identifiers, and HTTP(S) URL
fields remain usable. Sender-defined card IDs, thread keys, client message IDs
and resource names containing client message IDs stay wrapped. A usable URL is
data, not a recommendation to fetch it or trust its contents. Malformed metadata
strings are wrapped too. Classification is scoped
to full Chat output and survives `--results-only` and dotted `--select` paths;
compact output keeps its existing mention/reaction filtering. Without wrapping,
SDK-known strings are emitted unchanged.

## Replace text and delete

```bash
# Preview without credentials or a Google request.
gog chat messages update spaces/AAA/messages/BBB --text='Replacement' --dry-run --json
gog chat messages delete spaces/AAA/messages/BBB --delete-replies --dry-run --json

# Execute only for the intended account and message.
gog --account you@example.com chat messages update spaces/AAA/messages/BBB --text='Replacement' --json
gog --account you@example.com chat messages delete spaces/AAA/messages/BBB --force --no-input --json
```

`update` (alias `edit`) requires the **presence** of `--text`. `--text=''` sends
an explicit empty text field; Google can reject clearing a message if its rules
require content. The command always uses PATCH with `updateMask=text`. It does
not send cards or attachments, accept arbitrary masks, create a missing message,
or edit app cards.

`delete` (aliases `rm`, `remove`, `del`) requires destructive confirmation.
Noninteractive execution, including `--no-input`, refuses without root `--force`.
Root `--force` only bypasses CLI confirmation. The separate `--delete-replies`
flag sets Google's API `force=true`, which deletes threaded replies as well.
Without it, API `force=false` is sent and Google refuses deletion of a message
with threaded replies. The JSON receipt is
`{ "deleted": "spaces/AAA/messages/BBB", "deleteReplies": false }` after success.
No success receipt is emitted for provider failures.

After deletion, Google can return HTTP 200 with `deleteTime` and deletion metadata
when you get the message, rather than HTTP 404. That object is a deleted-message
tombstone. Deleted replies can instead return an ambiguous HTTP 403; the smoke
test requires a successful scoped thread listing with the target absent before
accepting that response as deletion evidence. Markdown formatting can wrap hyperlink labels
across lines; the smoke test normalizes whitespace when comparing the label.

Existing Chat authorization includes `chat.messages`; no new scope is requested.
Google still enforces message ownership, authorization type, and Workspace
permissions. App-authenticated updates/deletions are restricted to messages
created by that app. A resource ID or CLI `--force` grants no additional Google
permission; permission and not-found errors are propagated. `--readonly` blocks
both mutations, and the `agent-safe` and `readonly` safety profiles allow get
and explicitly deny update/delete (including aliases). Dry runs make no Google
calls and never prompt.

## Verification boundary

Focused HTTP transport tests cover full/compact output boundaries, cards and
image/attachment metadata, paging, missing read metadata, resource normalization,
markup, cancellation, provider errors, wrapping/projection, exact PATCH bodies,
explicit empty text, confirmation, force separation, dry run, and readonly
blocking. These tests use fake services; they do not establish live Workspace
edit/delete permission or exercise real Google messages.
