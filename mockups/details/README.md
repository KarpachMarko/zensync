# Five compact detail mockups

Open `index.html`. This set preserves the chosen sidebar and playlist design, adds compact layouts and illustrates the confirmed multi-user policy. The original and revised sets remain unchanged.

Five focused screens, also available as standalone entry files:

1. `playlist.html`: smaller sleeve, compact ordered rows, retained placeholders, owner/shared labels.
2. `library.html`: a dense song table with selection, availability, personal favorites and acquisition/request actions.
3. `activity.html`: compact server work, personal scope and an administrator overview; pending approval is not a running download.
4. `requests.html`: administrator inbox and dedicated request detail; approval, decline reasons, member request history and withdrawal.
5. `people.html`: roles, linked Navidrome accounts and the independent Add music without approval grant. Members never receive file-deletion rights.

The external **Preview as** selector is a prototype-review tool. Alex is a sample administrator; Maya and Sam are members. It does not represent a production impersonation control. All authorization is simulated in the browser; actual authentication/authorization remains implementation-plan work.

Try:

- Preview Maya, open Library, select Dayvan Cowboy and request it. It stays Awaiting approval and starts no download.
- Preview Alex, open Requests and approve Maya's request. Queued acquisition appears in Activity. Approval is distinct from completed import.
- Approve Sam's request for the same recording; the prototype represents one shared acquisition with separate request decisions.
- Grant Sam Add music without approval in People, preview Sam and request Two Weeks. It can queue directly, but Account still shows file deletion Not permitted. Revoking that grant returns an unstarted permission-only acquisition to approval; explicit admin approvals are preserved.
- Favorites remain personal across account previews. Shared playlist viewers cannot open the owner's private source-import details.
- As Alex, Skip for now on Teardrop, resolve it later and use Simulate completed import. The playlist keeps its original entry order.

Ordinary library rows measure 44px at the checked desktop width and 49px at the checked phone width. Controls retain larger touch targets on coarse pointers. Density is achieved mainly through padding and layout, not smaller primary text.

The full Apple XML example remains reachable under Imports, including all 12 songs, 3 playlists and 2 songs outside playlists. Imports save metadata without starting downloads. All navigation uses dedicated hash routes rather than workflow dialogs.

Standalone HTML embeds CSS, JavaScript and SVG artwork; no server/build/network is required. Sample state uses its own local-storage key, separate from earlier mockups. Reset sample restores accounts and requests. No emails are sent, files deleted or music downloaded.

`design-plan.md` describes the design choices. `compact.css` and `details.js` are the maintained extension sources, inlined into the delivered HTML. `qa.js` is injected at DETAILS_QA for browser-executed checks of routes, overflow, row density, personal favorites, request visibility, approval gating, grant revocation, acquisition deduplication, decline reasons and placeholder fulfillment. Visual checks cover 1000px desktop, 728px inline and 390px phone layouts.
