# Mention Notifications — Requirements Analysis

- **Feature ID:** mention-notifications
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-12

## 1. Competitor Research Summary

| Platform | Mention pattern | Key takeaways |
|---|---|---|
| **GitHub (issues/discussions)** | `@username` with autocomplete popup; unique immutable-ish usernames; mentions linkify to profiles; max ~10 notifications per comment; editing re-notifies only newly added mentions | Unique handles are the foundation — everything else builds on them; autocomplete is expected, not optional; edit semantics (only new mentions notify) prevent notification spam on edits |
| **Slack** | `@username` / `@display name` fuzzy autocomplete; member directory as source; `@here`/`@channel` broadcast mentions | Autocomplete sources the *member directory* — a bounded, searchable list; broadcast mentions are a separate, more dangerous class (we skip them) |
| **Twitter/X** | `@handle` in posts; notifications tab groups mentions; settings allow muting mentions from non-followers | Mention notifications are the primary re-engagement loop; mute-mentions-from-strangers is a later anti-abuse refinement |
| **Reddit** | `u/username` mentions create an inbox message; abused for harassment → later restricted to trusted users | Unrestricted mentions get abused; per-user rate limits on *emitted* mention notifications are the minimum guardrail |
| **Discourse** | `@username` autocomplete with full name fallback; "mentioned" filter in notifications; users can't opt out of being typed but can opt out of the notification | The distinction between *being mentionable* (anyone can type @you) and *receiving the notification* (preference-controlled) is the clean model |

**Synthesis:** The winning v1 pattern is the GitHub/Discourse model: (1) **unique usernames** — Open Prompts has none today (only non-unique `display_name`), so we add a `username` column, auto-provisioned at migration from display_name (slugified, collision-suffixed, `user-<shortid>` fallback for non-Latin names) and changeable in profile settings; (2) `@username` in comments with **autocomplete** in the composer (sourced from a new lightweight user-search endpoint returning only public fields); (3) server-side parsing on comment create/edit, notifying eligible mentioned users via a new `mention` notification type that slots into the shipped notifications system and the designed notification-preferences panel (data-driven, so the toggle appears automatically); (4) **eligibility rules** — no self-notify, public templates only (private: owner only), block-aware both directions (block-users FR-9), preference-suppressible at emission; (5) **anti-abuse** — max 10 unique mentions per comment (excess silently ignored, GitHub-style) and a per-author hourly cap on emitted mention notifications. Being *typed* in a comment is not controllable (GitHub model); *receiving* the notification is (preference toggle, default on).

## 2. Problem Statement

1. **No way to address anyone.** Comments shipped and replies are designed, but there is no mechanism to bring a specific person into a conversation. "Does @alice's template handle edge cases?" cannot reach Alice; conversations stay two-party (commenter ↔ owner).
2. **Users have no unique handle.** The `users` table has `email` (unique), `mobile` (unique), and `display_name` (NOT unique). There is nothing stable and public to type after an `@`. Any mention system without unique handles devolves into ambiguous display-name matching.
3. **The community loop is one-directional.** Notifications inform template owners of likes/favorites/forks/comments; a user who would benefit from a template discussion (e.g., a past commenter, a forked-template author) has no channel to be invited back.
4. **Mentions were explicitly deferred twice.** template-comments §Out-of-scope lists "@mentions and mention notifications"; comment-replies §Out-of-scope lists "@mentions inside reply text (plain text; a separate mention-notifications feature)". This is that feature.

## 3. Target Users

- **Commenters (primary):** users who want to pull a specific person into a template discussion — "hey @alice, you asked about this last week".
- **Mentioned users (primary):** users who get notified they were referenced and can jump straight to the exact comment.
- **Creators (secondary):** owners who want to credit or consult another creator in their template's comment section.
- **All users (tertiary):** everyone gains a public username and a profile handle, which follow-creators profiles and future social features build on.

## 4. User Stories

- **US-M1:** As a commenter, I want to type `@` and pick a user from autocomplete, so that addressing them is effortless and typo-free.
- **US-M2:** As a mentioned user, I want a notification with the commenter's name, the template title, and a snippet, so that I understand the context before clicking.
- **US-M3:** As a mentioned user, I want the notification to deep-link to the exact comment (anchored), so that I land in the right place in a long thread.
- **US-M4:** As a user, I want a unique username that I can change in profile settings, so that my handle is stable, public, and mine.
- **US-M5:** As a commenter, I want mentions in my comment to render as links to the mentioned user's profile, so that readers can discover who I referenced.
- **US-M6:** As a user, I want to turn off mention notifications in my notification preferences, so that being typed by others never disturbs me.
- **US-M7:** As a commenter editing my comment, I want only *newly added* mentions to notify, so that edits never re-ping everyone.
- **US-M8:** As a user who blocked someone (or was blocked), I want no mention notifications between us in either direction, so that blocking fully severs the channel.
- **US-M9:** As a commenter, I want to mention several users in one comment (up to 10), so that I can address a group without spamming one-by-one.
- **US-M10:** As the owner of a private template, I want mentions on my private template to notify only me (the owner), so that private content never leaks a notification to unauthorized users.
- **US-M11:** As a user with a non-Latin display name, I want an auto-generated usable username, so that I am mentionable from day one.
- **US-M12:** As an anonymous visitor, I want @mentions in comments to render as profile links (no interaction needed), so that public comment threads are readable logged-out.

## 5. Functional Requirements

- **FR-1:** **Usernames.** Add `username TEXT UNIQUE NOT NULL` to `users`. Migration auto-provisions: slugify display_name to `[a-z0-9-]`, trim to 3–30 chars, strip leading/trailing hyphens; if empty (non-Latin names) use `user-<first 8 chars of id>`; on collision append `-2`, `-3`, … Users may change their username in Profile settings: 3–30 chars, `^[a-z0-9-]+$`, no leading/trailing hyphen, uniqueness checked (409 on taken), rate-limited to one change per 7 days; old usernames are not reserved.
- **FR-2:** **Mention syntax & parsing.** A mention is `@username` where username matches `^[a-z0-9-]{3,30}$`, preceded by start-of-text or a non-word character. Parsing is server-side on comment create and edit (top-level comments and replies alike). Max **10 unique** mentions per comment; excess mentions render as plain text and never notify (silent, GitHub-style). The parser is case-insensitive on lookup (usernames are stored lowercase).
- **FR-3:** **Eligibility.** A mention creates a notification only if the mentioned user: exists; is not the comment author (`no_self_notify`); has mention notifications enabled (preferences, default on); is not blocked in either direction relative to the author (block-users FR-9 semantics); and can view the template — public templates notify anyone; private templates notify only the template owner. Banned mentioned users receive nothing.
- **FR-4:** **Notification record.** New type `mention` (extend `notifications.type` CHECK to `('like','favorite','fork','comment','comment_reply','mention')`). Add nullable `comment_id UUID REFERENCES template_comments(id) ON DELETE SET NULL` to `notifications` for deep-linking (existing rows null). Payload: actor (commenter) id/display-name snapshot, template id/title (live join), comment snippet (first 100 chars), `comment_id`. Click → `/templates/{id}#comment-{comment_id}`.
- **FR-5:** **Edit semantics.** On comment edit (within the shipped 15-minute window), diff the mention sets: only mentions **added** by the edit notify. Removed mentions do not un-send (notifications are immutable history). Re-adding a previously removed mention in a later edit notifies again only if that user was not already notified by this comment (per-comment per-recipient dedup).
- **FR-6:** **Anti-abuse.** Per-author cap: at most 20 mention notifications emitted per rolling hour (excess silently suppressed; the comment still posts and mentions still render). Per-comment cap per FR-2. User-search endpoint (FR-8) rate-limited to 60 requests/min per user.
- **FR-7:** **Rendering.** Comment content rendering (which today escapes text client-side) additionally linkifies valid `@username` mentions to `/creators/{user_id}` (profile route per follow-creators design). Linkification runs after escaping, matching only the mention pattern; unknown usernames render as plain text. Applies to comments and replies, for all viewers including anonymous.
- **FR-8:** **User search (autocomplete source).** New endpoint `GET /api/v1/users/search?q={prefix}` (auth required): returns max 10 users `{id, username, display_name, avatar}` whose `username` or `display_name` starts with the prefix (min 2 chars, case-insensitive). Never returns email, mobile, or status. Ordered: exact username match first, then username prefix, then display-name prefix.
- **FR-9:** **Composer autocomplete.** The CommentComposer (and reply composer per comment-replies design) triggers an autocomplete dropdown when the user types `@` followed by ≥2 characters: keyboard-navigable (↑/↓/Enter/Escape), click-selectable, showing avatar, username, and display name. Inserting a completion replaces the typed prefix with the exact `@username` followed by a space.
- **FR-10:** **Preferences integration.** The `mention` type appears in the notification-preferences panel (data-driven type list) with a plain-language description, default on. Suppression is at emission (no row created), consistent with the preferences design.
- **FR-11:** **Profile integration.** The Profile page shows the user's username (editable per FR-1 rules) with an inline availability check (debounced) and a link preview of `/creators/{id}`. The username is displayed on the creator profile page next to the display name.
- **FR-12:** **Localization.** The notification string ("{{actor}} mentioned you in a comment on {{template}}"), autocomplete labels, settings copy, and errors exist in EN and ZH from day one.

## 6. Non-Functional Requirements

- **NFR-1:** **Zero regression for existing comments.** All existing comment content renders exactly as today (no `@` sequences exist in the wild that match the username pattern after migration — and if any do, they linkify harmlessly to a profile or stay plain text if the username doesn't exist).
- **NFR-2:** **Performance.** Mention parsing is a single regex pass over ≤2000 chars. Notification inserts reuse the existing best-effort pattern (never block the comment transaction on failure). User search hits a new index on `users(username)`; p95 < 50 ms.
- **NFR-3:** **Privacy.** Usernames are public; emails and mobiles never leave the search endpoint. Mention notifications never include comment content beyond the 100-char snippet, and only for users eligible to view the template.
- **NFR-4:** **Accessibility.** The autocomplete dropdown implements the ARIA combobox pattern (`role="listbox"`/`option`, `aria-expanded`, `aria-activedescendant`); mention links are standard anchors; notification rows reuse the shipped notifications a11y.
- **NFR-5:** **i18n.** All strings EN/ZH; usernames themselves are ASCII-only by construction (no locale issues); the `@` symbol is universal.
- **NFR-6:** **Consistency.** Mention notifications appear in the bell dropdown, history page, unread count, and mark-read flows exactly like existing types — no special-casing in the UI layer beyond the type label and deep-link anchor.

## 7. Acceptance Criteria

- **AC-1:** After migration, every user has a unique non-null username; two users named "Alex Chen" get `alex-chen` and `alex-chen-2`; a user with display name "王小明" gets `user-<id8>`.
- **AC-2:** Typing `@al` in the comment composer shows a dropdown including `alex-chen (Alex Chen)`; selecting it inserts `@alex-chen `; posting the comment renders `@alex-chen` as a link to Alex's profile.
- **AC-3:** Alex receives a `mention` notification with the commenter's name, template title, a ≤100-char snippet, and clicking it navigates to `/templates/{id}#comment-{commentId}` and scrolls to the comment.
- **AC-4:** A comment containing 12 unique mentions notifies only the first 10 (parse order); the comment posts successfully and all 12 render as links.
- **AC-5:** Editing a comment within 15 minutes to add one new mention notifies only that user; editing to remove a mention sends nothing; re-adding it in a second edit sends nothing (per-comment dedup).
- **AC-6:** A user who disabled `mention` in preferences receives no row (suppressed at emission); the bell count and history stay clean.
- **AC-7:** If A blocked B (either direction), B's comment mentioning A creates no notification for A, and A's comment mentioning B creates none for B.
- **AC-8:** A mention on a private template notifies only the template owner; mentioning a non-owner on a private template renders the link but sends nothing.
- **AC-9:** Mentioning yourself (`@own-username`) renders as a link but creates no notification.
- **AC-10:** The 21st mention notification within an hour from the same author is silently suppressed; the comment and its rendered mentions are unaffected.
- **AC-11:** Changing username in Profile settings to a taken name shows an inline "already taken" error (409); a valid change updates the profile, and old comments mentioning the previous username render as plain text (no broken links).
- **AC-12:** All new strings exist in EN and ZH; the autocomplete is fully keyboard-operable; mention notifications read correctly in both locales.

## 8. Out of Scope (v1)

- Mentions anywhere outside comments/replies (template descriptions, titles, collection names).
- Broadcast mentions (`@here`, `@all`, `@channel`).
- Mention muting per-source (mute mentions from non-followers — Twitter-style refinement).
- Email/push delivery of mention notifications (in-app only, consistent with the notifications system).
- Username reservation or claim windows after a change.
- Mention analytics (who mentions whom most).
- Rich mention cards (hover previews of the mentioned user).
- Admin visibility into mention abuse beyond the existing comment moderation (admin can already delete comments).

## 9. Dependencies & Assumptions

- **D-1:** `template_comments` (init.sql:290-301) and the shipped comments service/components (CommentsSection, CommentComposer, CommentItem) exist; comment-replies-threads (design) adds `parent_comment_id` and the `comment_reply` type — this feature composes with both (mentions work in replies too).
- **D-2:** `notifications` (init.sql:260-276) with type CHECK and `no_self_notify`; the notifications UI (bell, history, mark-read) is type-agnostic except for labels. Assumes widening the CHECK and adding nullable `comment_id` is acceptable.
- **D-3:** notification-preferences (design) defines a data-driven five-type list with suppression at emission; assumes `mention` slots in as a sixth type without schema friction (the preferences table is column-per-type — a `mention_enabled` column is added by that feature's implementation or this one; flagged in §10).
- **D-4:** block-users (design) FR-9 defines bidirectional notification suppression; assumes its `user_blocks` table and helper are available at emission time.
- **D-5:** follow-creators (design) defines the `/creators/:id` profile route; mention links target it. If that feature is not yet shipped, links fall back to no-op (plain text) — the renderer checks route availability.
- **D-6:** Assumes the architect confirms the username provisioning strategy (FR-1) — in particular auto-provisioning at migration vs. lazy provisioning on first profile visit.

## 10. Open Questions for Architect

1. **Username provisioning:** auto-provision all users at migration (design's choice — everyone is mentionable immediately) vs. lazy (username assigned on first profile edit; un-provisioned users unmentionable)? Auto is better UX but writes to every user row once.
2. **`comment_id` on notifications:** nullable FK with `ON DELETE SET NULL` — acceptable widening? Alternative: encode the anchor in a JSON `meta` column, but that's a bigger departure from the current shape.
3. **Preferences column ownership:** notification-preferences (design) specifies a column-per-type table (`like_enabled`, …, `follow_enabled`). Should `mention_enabled` be added by that feature's migration or this one's? (Sequencing question only.)
4. **User search endpoint shape:** `GET /api/v1/users/search` vs. extending an existing user RPC — the proto has no user-search today. Any preference on placement (prompt.proto UserService vs. a new UserSearchService)?
5. **Mention cap enforcement point:** parse-order-first-10 (design's choice, deterministic) vs. notify-all-then-cap (surprising). Confirm 10/comment and 20/hour feel right.
6. **Username change ripple:** old mentions rendering as plain text after a rename is accepted (GitHub does the same via redirect; we skip redirects in v1). Confirm no redirect table is wanted.
