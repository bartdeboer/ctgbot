# Timed intents (schedulerv2)

This is the proposed foundational model for everything time-driven in ctgbot:
heartbeat ticks, turn-based wakeups, cron schedules, theater pull-notices, and
provider/runtime maintenance such as Claude prompt-cache keepalive.

It builds on `wakeup-architecture.md` and resolves its open questions. Where
this document disagrees with an assumption in that one, the disagreement is
called out explicitly in "Corrections to wakeup-architecture.md".

## Design stance

One persisted entity, one delivery layer, several thin writers.

```text
writers:   heartbeat | turn wakeup | cron | maintenance registration
entity:    TimedIntent (durable, per purpose, per thread)
delivery:  due intents -> per-thread coalescing -> one wake turn
                       -> per-intent maintenance handlers
```

There is no "wake" entity separate from a "job" entity. A one-shot wake, a
standing heartbeat, a cron schedule, and a bounded keepalive are the same row
with different schedule and delivery fields. Everything else is policy code
that writes rows.

## The entity: TimedIntent

```text
TimedIntent
  // identity
  ID              uuid
  TargetThreadID  uuid      // thread affected or woken; nullable for global maintenance
  Kind            string    // heartbeat | wake | cron | maintenance
  Key             string    // "default", "morning-prep", "claude:cache-keepalive", ...

  // ownership
  OwnerThreadID   uuid      // thread that created the intent (null for operator/CLI)
  OwnerActorID    string    // actor that created it, for audit

  // schedule
  NextDueAt       *time     // derived state: next delivery
  Every           string    // interval recurrence, e.g. "30m"
  Cron            string    // cron recurrence, standard syntax
  Timezone        string    // cron timezone (CRON_TZ rules from scheduler-cron)
  ExpiresAt       *time     // hard stop for bounded recurrence
  MaxRuns         int       // 0 = unbounded, n = stop after n deliveries
  RunCount        int

  // delivery
  Delivery        string    // turn | maintenance
  HandlerRef      string    // maintenance only: component ref that handles it
  ParamsJSON      string    // maintenance only: small handler params
  Label           string    // short human/agent-readable purpose, used in reasons

  // state
  Enabled         bool
  LastRunAt       *time
  LastStatus      string    // never | success | failed | expired
  LastError       string
```

Notes:

- **There is no one-shot type.** A one-shot is an intent with `NextDueAt` set
  and no recurrence (`Every`/`Cron` empty). After delivery no next due time can
  be computed, so the intent completes. `MaxRuns=1` with recurrence behaves the
  same way. No special-casing anywhere.
- **There is no separate heartbeat config.** The standing heartbeat intent *is*
  the config: `Kind=heartbeat, Every=30m, Enabled=true`. `heartbeat stop`
  deletes or disables the row.
- **Intents store labels, not content.** Reasons are composed at delivery time
  (see below). Nothing in this table ever contains copied theater posts or
  message bodies.

## Identity and replacement

The replacement identity is `(TargetThreadID, Kind, Key)`. Writing an intent
upserts on that triple. This produces exactly the override semantics we need,
with no precedence table:

| writer | Kind | Key | replacement behavior |
|---|---|---|---|
| `turn config set wakeup 20m` | `wake` | `default` | last write in a turn wins |
| `heartbeat start 30m` | `heartbeat` | `default` | replaces standing heartbeat |
| `scheduler add morning-prep --cron "0 8 * * *"` | `cron` | `morning-prep` | named; independent of other cron intents |
| Claude cache keepalive | `maintenance` | `claude:cache-keepalive` | component replaces its own intent |

Cron intents replace only by name. Heartbeat replaces heartbeat. A turn wake
replaces the previous turn wake. A maintenance intent replaces the same
maintenance purpose. Nothing ever implicitly overwrites a different kind.

Replacement and coalescing are different mechanisms and must not share a key:
replacement identity is `(target, kind, key)` at write time; coalescing
boundary is `(target thread)` at delivery time.

## Schedule semantics

Stepping rules carried over from the scheduler-cron work, which got these
right:

- recurrence steps from **delivery time**, never from the original due time —
  no catch-up storms; a thread that was down for a day gets one delivery, not
  twenty-four;
- cron via a standard parser (`robfig/cron`, `CRON_TZ` support, `--tz` and
  `CRON_TZ` mutually exclusive);
- intents are persisted rows restored by reading the table — restart recovery
  is free.

Bounds are checked at delivery: if `RunCount >= MaxRuns` (when `MaxRuns > 0`)
or `now >= ExpiresAt`, the intent is marked `expired` instead of delivered.
Expiry of a maintenance intent is reported to its handler (one final call with
an expired flag) so components can run a closing action — for cache keepalive,
"stop keeping warm, optionally compact".

### Timer precision

The current scheduler polls on a fixed 1-minute tick. That is too coarse for
maintenance intents: a Claude cache keepalive at 4m55s lands on whatever tick
boundary follows, which defeats its purpose against a 5-minute cache TTL.

schedulerv2 should sleep until the earliest `NextDueAt` (recomputed whenever an
intent is written), with the 1-minute poll retained only as a safety net. This
is also less idle load than fixed polling. Minimum interval policy stays for
`Delivery=turn` (attention has a floor; one minute is fine); maintenance
intents may go below it.

## Delivery modes

### `turn` — attention wakes

Due turn-intents become one inbound wake turn through the normal broker
pipeline: an inbound message with `provider=wakeup` and a prompt context
listing reasons. Same entry path as internal thread messages — never a special
direct call into an agent component, and never an outbound status message.

Reasons are composed **at delivery time**. The intent contributes its
`Kind`/`Key`/`Label`; heartbeat delivery additionally collects `UpdateFeed`
notices (theater unread counts and friends) at that moment, so badges are
fresh, not snapshots from scheduling time.

```text
[Wakeup]
Reasons:
- wakeup: you asked to check the build
- heartbeat: theater qwen-parser-lab (3 messages)
- cron: morning-prep
```

### `maintenance` — provider/runtime work

Due maintenance intents call a handler on the owning component, identified by
`HandlerRef`. They never produce a visible message and never start an agent
turn. Handler errors are recorded on the intent (`LastStatus=failed`) and do
not affect any other intent — poison isolation is per intent, not per loop.

Components register maintenance handlers the same way they implement other
optional interfaces:

```go
type MaintenanceHandler interface {
    Component
    HandleMaintenance(ctx context.Context, intent MaintenanceIntent) error
}
```

### There is no `command` delivery

The current scheduler runs stored argv as a root actor. That model should not
carry into schedulerv2, because it is both the authorization problem flagged in
review (any agent can schedule indirection to root) and a workflow-shaped
abstraction (canned commands) where ctgbot wants autonomy-shaped ones.

The replacement is structural, not a patch:

- **wakes carry information, not authority.** A cron intent for "morning-prep"
  does not run a privileged command; it wakes the agent with a reason, and the
  agent acts with its own command surface and its own permissions.
- **maintenance carries component authority.** A handler runs as its owning
  component, scoped to the target thread — not as a root actor interpreting
  argv.

The one existing `SourceScheduler` command consumer (indexing) becomes a
maintenance handler owned by the indexing component. If a genuine need for
scheduled raw commands ever appears, it can return as an operator-only feature;
nothing in this model depends on it.

## Coalescing

The coalescing rule, in full:

1. Delivery groups due `turn`-intents by `TargetThreadID`.
2. If the thread has no pending or running turn, all of its currently-due
   intents merge into **one** wake turn; each contributes one reason line.
   Their recurrences step; one-shots complete.
3. If the thread *does* have a pending or running turn, its due intents are
   simply left due. They merge into the next delivery attempt after the thread
   is free. No queue of stale wake turns exists, because no queue exists at
   all — "due and not yet delivered" is the only pending state.

Rule 3 is the implementation's whole skip-if-busy and anti-flooding story: a
1-minute heartbeat against a 5-minute agent turn produces one wake after the
turn, not five queued ticks. The pull model survives noisy boards because more
reasons never force more turns than the thread's attention allows.

Maintenance intents do not coalesce and do not care about turn state — a cache
keepalive must fire while a turn is running, since that is exactly when the
cache is warm and worth keeping.

## Heartbeat is an idle floor, not a chain

This is the one place this proposal corrects the model in
`wakeup-architecture.md`, which says: *"After a turn ends: 1. if the agent set
an explicit next wakeup for this cycle, honor that; 2. otherwise schedule the
next heartbeat wake."*

That phrasing makes the heartbeat's next occurrence depend on a turn-end hook
firing — a softer version of anti-shape 2. If the hook is skipped (crashed
turn, killed container, future refactor that forgets the hook), the chain
breaks silently, which is precisely what heartbeat exists to catch.

Instead, define heartbeat as an **idle floor**: the standing heartbeat intent
means "this thread must not go unattended longer than `Every`". Operationally:

- the heartbeat intent's `NextDueAt` is reset to `now + Every` whenever the
  thread receives attention — any completed turn (user message, internal
  thread message, delivered wake, anything);
- creating a turn wake at time `T` also pushes the heartbeat's `NextDueAt` to
  at least `T + Every`, because attention is already scheduled.

Two things fall out and replace the precedence rule entirely:

- **"explicit beats default" is automatic.** A turn wake at 20m with a 30m
  heartbeat: the wake fires at 20m, the turn resets the floor, next heartbeat
  at 50m. No double tick, no suppression flag, no cycle accounting.
- **conversations silence the heartbeat.** A thread actively talking to its
  human resets the floor on every turn; heartbeat only ever fires into
  silence. That is the correct meaning of a liveness floor.

A turn wake deliberately set *beyond* the floor (wake in 12h, heartbeat 30m)
pushes the floor out: the agent explicitly chose its next attention point, and
the wake itself is durable and system-delivered, so liveness is preserved. The
floor protects against *no wake scheduled*, not against *wake scheduled far*.

The reset is maintained by the delivery/turn layer (broker observes turn
completion), not by agent code and not by a per-turn policy hook that writes
new rows. The standing intent row never disappears; only its clock moves.

## Authorization and ownership

- Every intent is stamped with `OwnerThreadID` and `OwnerActorID` at creation.
- **Default scope is self.** An agent may create, list, replace, and remove
  intents where `TargetThreadID` equals its own thread. That covers heartbeat,
  turn wakes, own cron schedules, and component maintenance for the thread.
- **Cross-thread intents are operator/root-only.** If agent A believes thread
  B should wake, A sends B a direct message (the existing push path) and B
  decides its own schedule. This keeps attention sovereignty with the thread
  that pays for the turns — the same principle as theater's pull model.
- `wakeup list` shows the current thread's intents; a root/operator variant
  may list all. Other threads' schedules are not visible to agents, both for
  privacy and to remove the "guessable name, global remove" failure mode the
  scheduler-cron review found (`heartbeat:<threadID>` deletable by anyone).
- Delivery authority is described under "There is no `command` delivery":
  turn wakes carry none; maintenance runs as the owning component.

## Worked examples

Heartbeat, 30 minutes:

```text
Kind=heartbeat Key=default Target=T Owner=T Every=30m Delivery=turn
NextDueAt resets to now+30m on every completed turn for T.
At delivery: collect UpdateFeed notices, one reason line per notice.
```

Turn wakeup:

```text
turn config set wakeup 20m
-> upsert Kind=wake Key=default Target=T Owner=T NextDueAt=now+20m
   Label="you asked to check the build" (optional reason argument)
One-shot: no recurrence, completes after delivery.
Also pushes heartbeat NextDueAt to >= due+30m.
```

Cron, morning prep:

```text
Kind=cron Key=morning-prep Target=T Owner=T Cron="0 8 * * *" Timezone=Europe/Amsterdam
Delivery=turn, Label="morning-prep"
Wakes the agent with reason "cron: morning-prep"; the agent does the work
with its own permissions.
```

Claude prompt-cache keepalive, bounded to one hour:

```text
Kind=maintenance Key=claude:cache-keepalive Target=T Owner=T
Every=4m55s ExpiresAt=now+1h Delivery=maintenance HandlerRef=claude/claude
Handler pings the provider to keep the cache warm. On expiry the handler
gets a final expired call and may compact the thread. Requires
sleep-until-due timer precision; exempt from the turn-delivery minimum
interval; fires regardless of turn state.
```

Theater pull-notice:

```text
No intent of its own. Theater stays an UpdateFeed; heartbeat delivery asks
it for fresh unread counts. Theater never wakes anyone.
```

## Corrections to wakeup-architecture.md

Stated plainly, as requested:

1. **Heartbeat scheduling via turn-end decision is the wrong shape.** See
   "Heartbeat is an idle floor". The turn-end hook formulation reintroduces
   chain fragility; the idle-floor formulation also deletes the precedence
   rule as a separate mechanism.
2. **"Coalescing key, most likely thread ID" conflates two keys.** Replacement
   identity is `(target, kind, key)` at write time; coalescing is per target
   thread at delivery time. They are different mechanisms with different keys.
3. **One-shot vs recurring is not a type split.** A one-shot is an intent
   without a recurrence rule. Modeling it as a separate category would
   special-case the most common write path (`turn config set wakeup`).
4. **The open question "notice generation at scheduling or delivery time" has
   a definite answer: delivery time.** Scheduling-time badges go stale across
   coalescing and idle periods; theater unread counts are only meaningful at
   the moment of the wake.
5. **Carrying the current scheduler's argv-as-root jobs into v2 would be a
   mistake** — not because it needs better guards, but because wakes that
   carry information instead of authority make the guards unnecessary. The
   ownership questions in the current doc mostly dissolve under this rule.

## Migration notes

- `ScheduledJob` rows map onto `TimedIntent`: heartbeat jobs become
  `Kind=heartbeat` intents; the indexing command job becomes a maintenance
  handler registration; nothing else exists in the wild (single instance).
- The scheduler component's `job add/list/remove` surface becomes the intent
  surface with self-scoping; `heartbeat start/stop/status` keeps its UX and
  writes the standing intent underneath.
- `RunDue`'s per-job error isolation issue (one failing `FinishJob` aborts the
  loop) does not carry over: delivery state is per intent by construction.
- The wake delivery path (`provider=wakeup` inbound turns) is the keystone and
  ships first; heartbeat moves onto it second; turn wakes third; cron and
  maintenance follow. Same slicing as wakeup-architecture.md, unchanged.

## Remaining open questions

- Should a turn wake accept an optional reason argument
  (`turn config set wakeup 20m --reason "check the build"`)? Cheap and
  probably worth it from the start.
- Should direct messages reset the heartbeat floor only on completed turns, or
  also on queued-but-unprocessed inbound? Completed turns is the conservative
  start.
- Does maintenance need per-handler concurrency limits (e.g. llama.cpp model
  loads behind workgate)? Likely reuse `workgate` inside handlers rather than
  modeling it in the scheduler.
- Visibility: should operators see a unified upcoming-intent timeline across
  threads (`ctgbot intents`)? Useful, root-only, not needed for v1.
