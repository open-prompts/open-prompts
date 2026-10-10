# Notifications — UI/UX Design

- **Feature ID:** notifications
- **Version:** 1.0
- **Status:** Proposed (for architecture review)
- **Author:** ui-ux-agent
- **Date:** 2026-10-10
- **Companion doc:** `notifications-requirements.md`

## 1. Design Principles

1. **Quiet until meaningful.** The bell is silent chrome until engagement exists; zero unread = no badge, no color, no motion. No notification about the user's own actions.
2. **Triage in place, manage in depth.** The dropdown answers "anything new?" in one click without leaving the page; the history page is where filtering, marking, and deleting happen.
3. **Aurora-native.** Reuse the header icon-button pattern, the profile-dropdown open/close pattern, existing Modal, Spinner, badges, toasts (NotificationContext), tokens, and light/dark themes. No new visual language.
4. **Poll, don't push.** 60s polling with visibility pause and backoff — no new infrastructure, no jumpy UI.
5. **Text is data, not markup.** Notification sentences are composed client-side from typed fields (type, actor, title) so they translate cleanly and can never inject HTML.

## 2. Information Architecture

```mermaid
flowchart LR
    A[Header] --> B[Bell icon + unread badge]
    B -->|click| C[Dropdown: 10 recent]
    C -->|item click| D[/templates/:id]
    C -->|View all| E[/notifications]
    C -->|Mark all read| F[POST read-all]
    E --> G[Filter: All / Unread]
    E --> H[Row actions: Mark read / Delete]
    E -->|row click| D
    E -->|Mark all read| F
    E -->|Load more| E
```

**Surfaces:**

| Surface | Access | Content |
|---|---|---|
| Header bell + badge | auth only | Unread count (polled) |
| Bell dropdown | auth only | 10 most recent notifications, Mark all read, View all |
| `/notifications` | auth (redirect to /login) | Full history: filters, pagination, mark read, delete |

No admin console surface (requirements FR-29/30: per-user feature, nothing to moderate).

## 3. User Flows

### 3.1 Passive discovery (polling)

```mermaid
sequenceDiagram
    participant U as User (any page)
    participant H as Header bell
    participant API as GET /api/v1/notifications/unread-count
    loop every 60s while tab visible & authed
        H->>API: (token)
        API-->>H: {unread_count}
        H->>U: badge updates (99+ cap); SR live region announces politely
    end
    Note over H: tab hidden → pause; focus/visible → immediate refresh
```

### 3.2 Triage via dropdown

```mermaid
flowchart TD
    A[Click bell] --> B[Dropdown opens, fetches 10 recent]
    B --> C{User action}
    C -- Click item --> D[Optimistic mark read] --> E[Close dropdown] --> F[/templates/:id]
    C -- Mark all read --> G[POST read-all] --> H[Badge → 0, list re-renders read]
    C -- View all --> E2[Close dropdown] --> I[/notifications]
    C -- Outside click / Esc / route change --> E2
```

### 3.3 Manage history

```mermaid
flowchart TD
    A[/notifications] --> B[Filter tabs: All · Unread]
    B --> C{Row action}
    C -- Row click --> D[Mark read + navigate to template]
    C -- Mark read btn --> E[Optimistic read; badge decrements]
    C -- Delete btn --> F[Confirm modal] --> G[DELETE; row animates out]
    H[Mark all read btn] --> I[POST read-all; badge → 0]
    J[Load more btn] --> K[Append next 20 via page_token]
```

## 4. Page Layouts

### 4.1 Header bell (all pages, authenticated)

```text
┌──────────────────────────────────────────────────────────────────────┐
│ [☰] [logo Open Prompts] [Prompt Square]      [🌙] [文A] [🔔³] [👤 ▾] │
└──────────────────────────────────────────────────────────────────────┘
                                                    ↑ icon-btn, badge ③
```

- Bell sits between the language switch and the profile button, using the existing `.icon-btn.header-icon-btn` style.
- Badge: 16px min circle, bottom-right of the icon, `--danger` bg, white text 10px; "99+" when > 99; hidden at 0.

### 4.2 Bell dropdown

```text
┌──────────────────────────────────────────────┐
│ Notifications                    Mark all read│
├──────────────────────────────────────────────┤
│ ● ❤ Alice liked your template                │
│     "Email writer" · 2h ago                  │
├──────────────────────────────────────────────┤
│ ● ★ Bob favorited your template              │
│     "SQL helper" · 5h ago                    │
├──────────────────────────────────────────────┤
│   ⎇ Carol forked your template               │
│     "Code review" · 1d ago                   │
├──────────────────────────────────────────────┤
│ ❤ Dave liked your template                   │
│     "Logo brief" · 3d ago                    │
├──────────────────────────────────────────────┤
│                   View all →                 │
└──────────────────────────────────────────────┘
```

- Width 360px (280px below 480px viewport), max-height 480px, internal scroll.
- `●` unread dot on the left; unread rows get `--surface-3` tint.
- Loading: 3 skeleton rows. Error: inline message + Retry. Empty: "No notifications yet" + short hint.

### 4.3 History page (`/notifications`)

```text
┌──────────────────────────────────────────────────────────────────────┐
│ Header (existing, bell present)                                      │
├────────────┬─────────────────────────────────────────────────────────┤
│ Sidebar    │  Notifications                                           │
│ (existing) │  ┌──────────────┬──────────────────┐ ┌────────────────┐ │
│            │  │ All (24)     │ Unread (3)       │ │ Mark all read  │ │
│            │  └──────────────┴──────────────────┘ └────────────────┘ │
│            │  ┌────────────────────────────────────────────────────┐ │
│            │  │ ● ❤ Alice liked your template "Email writer"       │ │
│            │  │    2h ago                        [Mark read][Del]  │ │
│            │  ├────────────────────────────────────────────────────┤ │
│            │  │   ★ Bob favorited your template "SQL helper"       │ │
│            │  │    5h ago                                  [Del]    │ │
│            │  ├────────────────────────────────────────────────────┤ │
│            │  │   ⎇ Carol forked your template "Code review"       │ │
│            │  │    1d ago                                  [Del]    │ │
│            │  └────────────────────────────────────────────────────┘ │
│            │  [ Load more ]                                          │
└────────────┴─────────────────────────────────────────────────────────┘
```

- Rows are the same visual pattern as the dashboard's manage rows: `--surface-2` card, `--border` separators, hover `--surface-3`.
- Filter tabs match the dashboard's DashboardNav style (text + 2px `--accent` underline on active) with live counts in parentheses.

## 5. Component Specifications

### 5.1 NotificationBell (header)

| Aspect | Spec |
|---|---|
| Placement | Header right cluster, between language switch and profile; rendered only when authenticated |
| Trigger | `.icon-btn.header-icon-btn` with `BellIcon` (new icon, 19px, stroke `currentColor`) |
| Badge | Absolute, bottom-right; min-width 16px, height 16px, `--radius-pill`, `--danger` bg, `#fff` 10px/600 text, 1px `--surface-1` ring for contrast; content `99+` cap; `display:none` at 0 |
| Data | `unreadCount` from `notificationsSlice`; polled per FR-10 |
| A11y | `aria-label` = "Notifications, {{n}} unread" (or "Notifications" at 0); `aria-haspopup="true" aria-expanded` toggled; badge count also pushed to a polite live region |

### 5.2 NotificationDropdown

| Aspect | Spec |
|---|---|
| Open/close | Same pattern as profile dropdown: click toggles; outside `mousedown`, Escape, and route change close; focus moves into the dropdown on open, returns to the bell on close |
| Header row | Title "Notifications" (`--text-1` 14px/600) + "Mark all read" text button (`--accent`, hidden when unread = 0) |
| List | 10 most recent (mixed read/unread), fetched on open; each row: unread dot (6px `--accent`), event icon, sentence, template title quoted, relative time (`--text-3` 12px) |
| Row click | Optimistic mark read → close → navigate `/templates/:id` |
| Footer | "View all" full-width text button → `/notifications` |
| Styling | `--surface-2`, `--border`, `--radius-12`, `--shadow-lg`; width 360px (280px < 480px viewport); rows 56–72px, `--border` separators |
| States | Loading: 3 skeleton rows (existing skeleton pattern); Error: message + Retry; Empty: icon + "No notifications yet" + hint text |

### 5.3 NotificationRow (history page)

| Aspect | Spec |
|---|---|
| Content | Unread dot · event icon · sentence ("{{actor}} liked your template \"{{title}}\"") · relative time (absolute in `title`) · actions |
| Event icons | like → `HeartIcon` `--danger`; favorite → `StarIcon` `--warning`; fork → `ForkIcon` `--secondary`; 16px, `aria-hidden` |
| Actions | "Mark read" (icon+text, only on unread rows) and "Delete" (`TrashIcon`, `--danger` on hover) |
| Row click | Same as dropdown row: mark read + navigate to template |
| Delete | Confirm modal (existing Modal, echoes the sentence) → DELETE → row animates out; if the deleted row was unread, badge decrements |
| Styling | Unread rows: `--surface-3` tint + dot; read rows: `--surface-2`, `--text-2`; hover `--surface-3` |

### 5.4 FilterTabs (All / Unread)

| Aspect | Spec |
|---|---|
| Content | "All ({{n}})" · "Unread ({{n}})" — counts from the latest list response |
| Behavior | Switching resets pagination and refetches; active tab `--text-1` + 2px `--accent` underline |
| A11y | `<nav aria-label="Notification filters">`; tabs are links with `aria-current` |

### 5.5 MarkAllRead button

| Aspect | Spec |
|---|---|
| Placement | History page header row (`.btn-ghost btn-sm`) and dropdown header (text button) |
| Behavior | POST read-all → optimistic badge → 0 and rows re-render as read; disabled while request in flight; hidden/disabled when unread = 0 |
| Feedback | Success toast (existing NotificationContext): "All notifications marked as read" |

## 6. Interaction Details

1. **Polling lifecycle:** starts on login/restore, 60s interval; `document.visibilitychange`/`focus` → immediate refresh when returning visible; hidden tab pauses; failures back off 60→120→300s and reset on success; logout stops the timer and zeroes state. All timing constants live in one module (`POLL_INTERVAL_MS` etc.).
2. **Optimistic reads:** mark-read/mark-all update `notificationsSlice.unreadCount` and the row immediately; server error reverts and shows an error toast.
3. **Dropdown fetch:** on open, `GET /notifications?page_size=10` (filter=all); response `unread_count` reconciles the badge (single source of truth: slice).
4. **History pagination:** "Load more" appends via `next_page_token`; hidden when token absent; filter switch resets the list.
5. **Navigation from rows:** rows are links to `/templates/:id`; if the template was deleted between event and click (404), the existing template-detail error state handles it — no special case.
6. **Relative time:** shared util (dashboard D-4/D-6): <60s "just now", <60m "Xm ago", <24h "Xh ago", <7d "Xd ago", else locale date; absolute timestamp in `title`.
7. **Sentence composition:** `t('notifications.event.like', {actor, title})` etc. — actor name and title are interpolated values, never translated; React escapes them (NFR-9).
8. **Toast vs notification:** toasts (existing `NotificationContext`) remain for action results ("Marked as read"); the bell/dropdown/history are the new persistent channel. No visual overlap: toasts stay top-right transient, notifications are badge + surfaces.

## 7. States Summary

| State | Trigger | UI |
|---|---|---|
| Badge = 0 | No unread | Bell without badge |
| Badge > 0 | Unread exists | Bell + count badge (99+ cap) |
| Dropdown loading | Open | 3 skeleton rows |
| Dropdown empty | No notifications at all | Icon + "No notifications yet" + hint |
| Dropdown error | Fetch fails | Message + Retry (badge keeps last value) |
| Page loading | Route mount | Skeleton rows (6) |
| Page empty (All) | Zero notifications | Empty state + hint (engagement actions listed) |
| Page empty (Unread) | All read | "You're all caught up." |
| Page error | Fetch fails | Error banner + Retry |
| Anonymous | No token | No bell; `/notifications` → /login |
| Confirm open | Delete | Existing Modal pattern |
| Offline/degraded | Poll failures | Badge frozen at last value; backoff; no page breakage |

## 8. Visual Style (Aurora compliance)

- **Bell:** inherits `.icon-btn` hover (`--surface-3`); badge `--danger` bg + white text, `--radius-pill`, 1px `--surface-1` ring.
- **Dropdown:** `--surface-2` + `--border` + `--radius-12` + `--shadow-lg`; rows separated by `--border`; unread tint `--surface-3`.
- **Event icons:** like `--danger`, favorite `--warning`, fork `--secondary` — same color mapping as the dashboard activity feed for cross-feature consistency.
- **History rows:** `--surface-2` cards, `--border` separators, hover `--surface-3`; unread dot 6px `--accent`; time `--text-3` 12px.
- **Filter tabs:** `--text-2` idle, `--text-1` + 2px `--accent` underline active — identical to DashboardNav.
- **Buttons:** Mark all read `.btn-ghost btn-sm`; Mark read icon+text; Delete `TrashIcon` with `--danger` hover.
- **Dark/light:** token-only styling; automatic in both themes; badge ring guarantees contrast on both.

## 9. i18n Considerations (EN/ZH)

New namespace `notifications.*` (distinct from the existing toast-related keys and from `NotificationContext`):

```json
{
  "notifications": {
    "bell_aria": "Notifications, {{count}} unread",
    "bell_aria_zero": "Notifications",
    "title": "Notifications",
    "mark_all_read": "Mark all read",
    "view_all": "View all",
    "mark_read": "Mark read",
    "delete": "Delete",
    "load_more": "Load more",
    "filter": { "all": "All ({{count}})", "unread": "Unread ({{count}})" },
    "event": {
      "like": "{{actor}} liked your template \"{{title}}\"",
      "favorite": "{{actor}} favorited your template \"{{title}}\"",
      "fork": "{{actor}} forked your template \"{{title}}\""
    },
    "empty": {
      "all": "No notifications yet — when someone likes, favorites, or forks your templates, you'll see it here.",
      "unread": "You're all caught up.",
      "dropdown": "No notifications yet"
    },
    "toast": { "marked_read": "Notification marked as read", "all_marked_read": "All notifications marked as read", "deleted": "Notification deleted" },
    "confirm": { "delete": "Delete this notification? This cannot be undone." },
    "error": { "fetch": "Failed to load notifications.", "mark": "Failed to mark as read.", "delete": "Failed to delete notification.", "retry": "Retry" }
  }
}
```

ZH mirror:

```json
{
  "notifications": {
    "bell_aria": "通知，{{count}} 条未读",
    "bell_aria_zero": "通知",
    "title": "通知",
    "mark_all_read": "全部已读",
    "view_all": "查看全部",
    "mark_read": "标为已读",
    "delete": "删除",
    "load_more": "加载更多",
    "filter": { "all": "全部（{{count}}）", "unread": "未读（{{count}}）" },
    "event": {
      "like": "{{actor}} 赞了你的模板\"{{title}}\"",
      "favorite": "{{actor}} 收藏了你的模板\"{{title}}\"",
      "fork": "{{actor}} fork 了你的模板\"{{title}}\""
    },
    "empty": {
      "all": "还没有通知 —— 当有人点赞、收藏或 fork 你的模板时，会显示在这里。",
      "unread": "你已全部看完。",
      "dropdown": "还没有通知"
    },
    "toast": { "marked_read": "通知已标为已读", "all_marked_read": "全部通知已标为已读", "deleted": "通知已删除" },
    "confirm": { "delete": "删除这条通知？此操作不可撤销。" },
    "error": { "fetch": "加载通知失败。", "mark": "标记已读失败。", "delete": "删除通知失败。", "retry": "重试" }
  }
}
```

Notes: ZH uses full-width parentheses in filter counts. "fork" is kept as the loanword "fork" (consistent with the existing UI, e.g., `template_details.fork_template`). Relative-time strings come from the shared `time.*` util keys. Actor names and template titles are interpolated user content, never translated.

## 10. Accessibility

- **Bell:** real `<button>` with dynamic `aria-label` (unread count included); `aria-haspopup="true"` + `aria-expanded`; keyboard focusable with visible focus ring.
- **Live region:** an `aria-live="polite"` region announces "N unread notifications" when the polled count increases; decreases are not announced (noise control).
- **Badge:** count is real text inside the label — screen readers hear it with the button name; color is never the only signal (the dot + tint also mark unread).
- **Dropdown:** focus moves in on open and returns to the bell on close; list is a semantic `<ul>` of links; Escape closes; outside click closes; `role="menu"`-free simple listbox-free pattern matching the existing profile dropdown.
- **History rows:** each row is a link (title in the accessible name); actions are separate focusable buttons with `aria-label`s; delete is destructive-labeled.
- **Filter tabs:** nav landmark + `aria-current="page"`; counts are text.
- **Contrast:** badge white-on-`--danger` ≥ 4.5:1 in both themes; all text AA.

## 11. Implementation Notes for Developer

1. **Redux:** new `notificationsSlice` (`unreadCount`, `lastFetchedAt`, `error`); thunk `fetchUnreadCount`; selectors consumed by bell, dropdown, and history page (D-8).
2. **Header:** add `NotificationBell` between the language switch and profile cluster in `Header.js`; render only when `user` is truthy; add `BellIcon` to `ui/Icons.jsx` (stroke icon, 19px default).
3. **Polling hook:** `useNotificationPolling()` — interval + visibilitychange + focus handlers + exponential backoff; mounted once in `Layout` (or Header) behind the auth check; cleanup on unmount/logout.
4. **Routes:** `/notifications` (lazy) + auth guard consistent with the dashboard feature's guard pattern.
5. **API client:** `listNotifications({filter, page_size, page_token})`, `getUnreadNotificationCount()`, `markNotificationRead(id)`, `markAllNotificationsRead()`, `deleteNotification(id)` in `services/api.js`.
6. **Dropdown:** extract the outside-click/Escape close logic of the profile dropdown into a small `useDismissable` hook and reuse it in both places (keeps behavior identical).
7. **Shared utils:** relative time (with dashboard feature), optimistic-update helper pattern already used elsewhere (visibility toggle).
8. **Testing:** unit tests for the polling hook (fake timers: interval, visibility pause, backoff) and slice reducers; component tests for NotificationBell (badge states, aria-label), NotificationDropdown (open/close, item click navigation, mark-all), NotificationRow (actions, optimistic states), FilterTabs; page test for `/notifications` (loading/empty/error/auth-redirect/pagination); i18n key parity test.
9. **Backend:** new `notifications` table + `NotificationService` (or PromptService extension — architect's call, requirements §10) + 5 REST routes; event inserts in `ToggleLikeTemplate` (on), `ToggleFavoriteTemplate` (on), and `ForkTemplate`; partial index `(recipient_id) WHERE read_at IS NULL` for the count; FK cascades per FR-6.
