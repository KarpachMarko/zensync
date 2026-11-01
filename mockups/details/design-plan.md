# Five compact detail mockups

Preserve the chosen design, not five new competing themes. This pass adds five focused review screens: compact playlist detail, dense library songs, server Activity, administrator Requests inbox/detail, and People & permissions.

Tokens: midnight #172033, navy #1c2940, slate #29384f, porcelain #f3f2f8, fog #adb8cb and rose #ecc7d9. Mint indicates available/approved work, amber pending decisions, blue active operations. Avenir Next/system sans for UI, Georgia only for the playlist title. Left align text, right align duration and counts.

Compact layout: [sidebar | breadcrumb/account / page]. Ordinary desktop song rows target 44px with 6px vertical padding; unresolved rows may grow for explanatory copy and Skip for now. Buttons become about 30–32px on desktop; phone/coarse-pointer controls keep comfortable targets. Cut padding before reducing font size. Activity rows use concise stage and origin lines, not identical dashboard cards.

Playlist: [smaller sleeve/source | title + ordered compact rows]. Library: [view tabs + search/filter | checkbox / song+artist / album / status / favorite / duration]. Activity: [My work / Server overview (admin) / status and type filters / compact jobs]. Requests: [pending states / requester / recording / approve or decline / full detail page]. People: [user / role / add without approval toggle / linked account / admin-only file deletion rule].

Multi-user scenario: sample Alex is admin; Maya and Sam are members. Shared base audio, personal favorites/playlists/imports. New acquisitions require admin approval unless Add music without approval is granted. That grant never permits deleting files. The prototype's external Preview as selector is explicitly a design-review tool, not a production account-impersonation feature.

Pre-build critique: keep Requests separate from running Activity; approval is not a completed download. A member's request must never call the old sample download action directly. Preserve wishlist/placeholder semantics, distinguish private collections from common files, and make role-dependent controls visible for review. All pages remain dedicated hash routes. Original prototypes remain untouched.
