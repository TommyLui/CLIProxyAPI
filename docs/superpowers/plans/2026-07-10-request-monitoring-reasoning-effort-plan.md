# Request Monitoring Reasoning Effort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist the actual upstream reasoning effort for each request, show it in request monitoring, and align the status header with its row badges.

**Architecture:** The existing runtime already places the translated upstream reasoning effort in `usage.Record.ReasoningEffort`. Extend the raw usage ledger event and SQLite `usage_events` table to persist that value, expose it through analytics JSON, then render it as a dedicated centered column in Management. Keep rollups unchanged because reasoning effort is only required for per-request monitoring.

**Tech Stack:** Go 1.26, SQLite, Gin management API, React 19, TypeScript 6, Sass, Bun test/build.

## Global Constraints

- Display the translated value actually sent to the upstream provider.
- Historical rows and requests without reasoning configuration display `-`.
- Preserve the 60-day retention behavior and all existing usage aggregation keys.
- Add `reasoning_effort` only to raw request events, not `usage_rollups`.
- Monitoring columns must remain stable and horizontally scroll on narrow viewports.
- Do not alter GPT-5.6 model behavior, access scopes, pricing, or request routing.

---

### Task 1: Capture Reasoning Effort In Usage Ledger Events

**Files:**
- Modify: `internal/usageledger/plugin_test.go`
- Modify: `internal/usageledger/types.go`
- Modify: `internal/usageledger/plugin.go`

**Interfaces:**
- Consumes: `usage.Record.ReasoningEffort string` and `usage.ReasoningEffortFromContext(context.Context) string`
- Produces: `usageledger.Event.ReasoningEffort string`

- [ ] **Step 1: Write failing plugin mapping tests**

Extend `TestPluginStoresMonitoringFieldsFromUsageRecord` with a record value and assertion:

```go
plugin.HandleUsage(ctx, coreusage.Record{
    Provider:        "codex",
    Model:           "gpt-5.6-terra",
    ReasoningEffort: "ultra",
    // existing fields remain unchanged
})

if row.ReasoningEffort != "ultra" {
    t.Fatalf("reasoning effort = %q, want ultra", row.ReasoningEffort)
}
```

Add a context fallback test:

```go
func TestPluginStoresReasoningEffortFromContextFallback(t *testing.T) {
    store := openTestStore(t)
    defer store.Close()
    now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
    plugin := NewPlugin(store, func() time.Time { return now })
    ctx := coreusage.WithReasoningEffort(context.Background(), "high")

    plugin.HandleUsage(ctx, coreusage.Record{
        Provider: "codex",
        Model: "gpt-5.6-sol",
        RequestedAt: now,
    })

    result, err := store.Analytics(context.Background(), AnalyticsRequest{
        FromMS: now.Add(-time.Minute).UnixMilli(),
        ToMS: now.Add(time.Minute).UnixMilli(),
        Include: AnalyticsInclude{EventsPage: &AnalyticsEventsPage{Limit: 10}},
    })
    if err != nil { t.Fatal(err) }
    if got := result.Events.Items[0].ReasoningEffort; got != "high" {
        t.Fatalf("reasoning effort = %q, want high", got)
    }
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run:

```bash
go test ./internal/usageledger -run 'TestPluginStoresMonitoringFieldsFromUsageRecord|TestPluginStoresReasoningEffortFromContextFallback' -count=1
```

Expected: compile failure because `Event` and `AnalyticsEventRow` do not yet expose `ReasoningEffort`.

- [ ] **Step 3: Add the event field and plugin mapping**

Add to `Event`:

```go
ReasoningEffort string
```

In `eventFromRecord`, normalize the record value and fall back to context:

```go
reasoningEffort := strings.TrimSpace(record.ReasoningEffort)
if reasoningEffort == "" {
    reasoningEffort = coreusage.ReasoningEffortFromContext(ctx)
}
```

Assign it in the returned event:

```go
ReasoningEffort: reasoningEffort,
```

- [ ] **Step 4: Leave the tests failing only at persistence**

Run the same command. Expected: tests now compile, but assertions fail because SQLite does not persist the new value yet.

- [ ] **Step 5: Commit the mapping boundary**

```bash
git add internal/usageledger/plugin_test.go internal/usageledger/types.go internal/usageledger/plugin.go
git commit -m "feat(usage): capture request reasoning effort"
```

---

### Task 2: Persist And Return Reasoning Effort

**Files:**
- Modify: `internal/usageledger/sqlite_store.go`
- Modify: `internal/usageledger/analytics.go`
- Modify: `internal/usageledger/types.go`
- Modify: `internal/usageledger/store_test.go`
- Modify: `internal/api/handlers/management/usage_analytics_test.go`

**Interfaces:**
- Consumes: `usageledger.Event.ReasoningEffort string`
- Produces: `usageledger.AnalyticsEventRow.ReasoningEffort string` serialized as `reasoning_effort`

- [ ] **Step 1: Add failing persistence and API assertions**

In a SQLite round-trip test, insert:

```go
Event{
    RequestID: "reasoning-effort-event",
    Timestamp: now,
    Provider: "codex",
    Model: "gpt-5.6-terra",
    ReasoningEffort: "max",
}
```

Query events and assert:

```go
if got := result.Events.Items[0].ReasoningEffort; got != "max" {
    t.Fatalf("reasoning effort = %q, want max", got)
}
```

Update `TestUsageAnalyticsEndpointReturnsUsageLedgerAnalytics` to insert `ReasoningEffort: "high"` and assert the decoded response contains `high`.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/usageledger ./internal/api/handlers/management -run 'ReasoningEffort|UsageAnalyticsEndpointReturnsUsageLedgerAnalytics' -count=1
```

Expected: assertions fail because the database and analytics query drop the value.

- [ ] **Step 3: Add the non-destructive SQLite migration**

Add the column to new databases:

```sql
reasoning_effort TEXT NOT NULL DEFAULT '',
```

Add it to `ensureUsageEventColumns`:

```go
{name: "reasoning_effort", def: "TEXT NOT NULL DEFAULT ''"},
```

Add `reasoning_effort` to both event INSERT statements and add `event.ReasoningEffort` in the matching argument position.

- [ ] **Step 4: Add analytics selection and JSON output**

Add to `AnalyticsEventRow`:

```go
ReasoningEffort string `json:"reasoning_effort,omitempty"`
```

Select and scan `reasoning_effort` immediately after `service_tier`, then map it in `buildAnalyticsEventsPage`:

```go
ReasoningEffort: event.event.ReasoningEffort,
```

- [ ] **Step 5: Run the focused tests and verify GREEN**

```bash
go test ./internal/usageledger ./internal/api/handlers/management -run 'ReasoningEffort|UsageAnalyticsEndpointReturnsUsageLedgerAnalytics' -count=1
```

Expected: PASS.

- [ ] **Step 6: Run the backend regression suite**

```bash
go test ./internal/usageledger ./internal/api/handlers/management ./internal/runtime/executor/helps ./sdk/cliproxy/auth -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit persistence and API changes**

```bash
git add internal/usageledger internal/api/handlers/management/usage_analytics_test.go
git commit -m "feat(usage): persist reasoning effort in request events"
```

---

### Task 3: Define Frontend Formatting And Stable Columns

**Files:**
- Create: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/pages/usageMonitoringColumns.ts`
- Create: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/pages/usageMonitoringColumns.test.ts`

**Interfaces:**
- Produces: `MONITORING_COLUMN_WIDTHS: readonly number[]`
- Produces: `formatReasoningEffort(value?: string | null): string`

- [ ] **Step 1: Write the failing Bun tests**

```ts
import { describe, expect, test } from 'bun:test';
import { formatReasoningEffort, MONITORING_COLUMN_WIDTHS } from './usageMonitoringColumns';

describe('request monitoring columns', () => {
  test('defines ten columns totaling 100 percent', () => {
    expect(MONITORING_COLUMN_WIDTHS).toHaveLength(10);
    expect(MONITORING_COLUMN_WIDTHS.reduce((sum, width) => sum + width, 0)).toBe(100);
  });

  test('formats actual upstream reasoning effort', () => {
    expect(formatReasoningEffort(' Ultra ')).toBe('ultra');
    expect(formatReasoningEffort('')).toBe('-');
    expect(formatReasoningEffort(undefined)).toBe('-');
  });
});
```

- [ ] **Step 2: Run the test and verify RED**

```bash
bun test src/pages/usageMonitoringColumns.test.ts
```

Expected: FAIL because the module does not exist.

- [ ] **Step 3: Add the minimal helper module**

```ts
export const MONITORING_COLUMN_WIDTHS = [8, 12, 14, 7, 14, 8, 11, 14, 7, 5] as const;

export const formatReasoningEffort = (value?: string | null) => {
  const normalized = value?.trim().toLowerCase() ?? '';
  return normalized || '-';
};
```

- [ ] **Step 4: Run the test and verify GREEN**

```bash
bun test src/pages/usageMonitoringColumns.test.ts
```

Expected: 2 tests pass.

- [ ] **Step 5: Commit the frontend contract**

```bash
git add src/pages/usageMonitoringColumns.ts src/pages/usageMonitoringColumns.test.ts
git commit -m "test: define monitoring reasoning effort columns"
```

---

### Task 4: Render Reasoning Effort And Fix Alignment

**Files:**
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/services/api/usageAnalytics.ts`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/pages/UsageAnalyticsPage.tsx`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/pages/UsageAnalyticsPage.module.scss`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/i18n/locales/zh-CN.json`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/i18n/locales/zh-TW.json`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/i18n/locales/en.json`
- Modify: `/Users/kogeki/dev/Cli-Proxy-API-Management-Center/src/i18n/locales/ru.json`

**Interfaces:**
- Consumes: API field `reasoning_effort?: string`
- Consumes: `MONITORING_COLUMN_WIDTHS` and `formatReasoningEffort`

- [ ] **Step 1: Add the response type**

```ts
reasoning_effort?: string;
```

Add it beside `service_tier` in `UsageAnalyticsEventRow`.

- [ ] **Step 2: Add a real colgroup and the new column**

Render before `<thead>`:

```tsx
<colgroup>
  {MONITORING_COLUMN_WIDTHS.map((width, index) => (
    <col key={`${index}-${width}`} style={{ width: `${width}%` }} />
  ))}
</colgroup>
```

Add the header after “提供商 / 模型”:

```tsx
<th className={styles.monitoringCenterColumn}>{t('usage_analytics.reasoning_effort')}</th>
```

Add the row cell in the same position:

```tsx
<td className={styles.monitoringCenterColumn}>
  <span className={styles.reasoningEffortBadge}>
    {formatReasoningEffort(row.reasoning_effort)}
  </span>
</td>
```

- [ ] **Step 3: Fix centering and widths**

Remove the nine `nth-child` width rules. Set `min-width: 1580px`, retain `table-layout: fixed`, and make centered content fill its cell:

```scss
.monitoringCenterColumn {
  text-align: center;
}

.monitoringStatusCell {
  width: 100%;
  align-items: center;
  align-content: center;
  justify-items: center;
}

.reasoningEffortBadge {
  display: inline-flex;
  min-width: 58px;
  min-height: 26px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--ops-panel-border);
  border-radius: 999px;
  background: color-mix(in srgb, var(--bg-secondary) 72%, transparent);
  color: var(--text-secondary);
  font-size: 11px;
  font-weight: 800;
  line-height: 1;
}
```

- [ ] **Step 4: Add detail and CSV output**

Insert `reasoning_effort` after `model` in CSV headers and rows. Add a detail item beside model:

```tsx
<DetailItem
  label={t('usage_analytics.reasoning_effort')}
  value={formatReasoningEffort(selectedEvent.reasoning_effort)}
/>
```

- [ ] **Step 5: Add translations**

Use:

```json
"reasoning_effort": "思考强度"
```

with `思考強度`, `Reasoning effort`, and `Уровень рассуждения` in the other locale files.

- [ ] **Step 6: Verify frontend tests and build**

```bash
bun test src/pages/usageMonitoringColumns.test.ts
bun run type-check
bun run lint
bun run build
```

Expected: all commands exit 0.

- [ ] **Step 7: Commit the UI**

```bash
git add src/services/api/usageAnalytics.ts src/pages/UsageAnalyticsPage.tsx src/pages/UsageAnalyticsPage.module.scss src/i18n/locales src/pages/usageMonitoringColumns.ts
git commit -m "feat: show reasoning effort in request monitoring"
```

---

### Task 5: End-To-End Verification And Deployment

**Files:**
- Verify only; no source changes expected.

**Interfaces:**
- Consumes: backend analytics JSON and Management `reasoning_effort` column.

- [ ] **Step 1: Run complete backend verification**

```bash
go test ./...
```

Expected: PASS with zero failing packages.

- [ ] **Step 2: Inspect repository state**

Confirm only known untracked `.codegraph/`, `dist/`, `design-qa.md`, and `docs/` artifacts remain outside commits.

- [ ] **Step 3: Push both repositories**

Push `CLIProxyAPI/main` and `Cli-Proxy-API-Management-Center/main` through the configured local proxy.

- [ ] **Step 4: Deploy with rollback assets**

Build a clean backend source archive from Git, build a versioned Docker image on the bastion host, back up `docker-compose.yml` and `management.html`, then recreate only `cli-proxy-api`.

- [ ] **Step 5: Verify migration and live data**

Confirm:

- SQLite contains `usage_events.reasoning_effort`.
- New requests return actual values such as `max` or `ultra`.
- Historical records show `-`.
- Status header and badges share the same center line.
- `gpt-5.6-terra` remains fully visible.
- 503 summaries remain single-line ellipsized.
- Narrow layouts scroll horizontally without overlap.
- Health endpoint and real `/v1/responses` traffic return 200.

- [ ] **Step 6: Record final deployed version**

Report backend and Management commit IDs, Docker image version, health status, and retained rollback filenames.
