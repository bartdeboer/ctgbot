# Wakeup architecture

ctgbot should facilitate autonomous agents without forcing a rigid workflow. The
wakeup architecture is the timing substrate for that: agents can be nudged back
into attention, but noisy collaboration should not turn into a push-message
explosion.

## Why this exists

Theater exists for multi-agent collaboration. Directly broadcasting every agent
message to every other participant does not scale: one message wakes several
agents, each reply wakes several more, and turn cost grows with the event rate
that the turns themselves create.

Theater changes the collaboration shape from push to pull:

- a theater is a shared board/thread where discussion accumulates;
- subscribers see counts and hints, not pushed content;
- each agent samples the board at its own cadence;
- one wake can synthesize five new posts into one considered response.

The heartbeat interval is therefore a damping coefficient on collaboration. It
bounds **when** attention is spent while theater bounds **where** collaboration
happens. Direct thread messages remain the urgent push path when an agent needs
to be interrupted now.

## Core primitive

There should be one timer primitive:

```text
wake(thread, at, reason)
```

A wake is a durable one-shot intent: at time `T`, start an inbound turn for
thread `X` with reason `R`.

Durable means wake intents survive ctgbot restarts and container refreshes. They
belong in persisted scheduler state, not in an agent container or process-local
memory.

Delivery uses the normal broker turn pipeline. A wake becomes an inbound message
with a provider such as `wakeup` and a prompt context containing the reasons.
It is not an outbound status message to Telegram first; the whole point is to
wake the agent.

Example delivered prompt:

```text
[Wakeup]
Reasons:
- heartbeat: theater qwen-parser-lab (3 messages)
- cron: morning-prep
- wakeup: you asked to check the build
```

### Attention wakes vs maintenance intents

Most wake intents are attention wakes: they exist to start an inbound turn for
an agent.

Some future timed work may be thread-scoped but not agent-facing. For example,
Claude prompt-cache keepalive may want to call the provider every few minutes to
keep the cache warm for a bounded window, then compact the thread and stop.

That should still use the same persisted scheduler substrate, but it should not
be forced through the human/agent-visible wake message path if the work is
really provider maintenance.

The broader primitive is therefore:

```text
timed_intent(thread, at, kind, reason, delivery)
```

Where `delivery` can start with:

- `turn` — enqueue an inbound wake turn;
- `maintenance` — call a component/runtime maintenance handler.

The first implementation can focus on `turn`. The storage and naming should
avoid assuming that every timed intent must become a visible message.

## Three writers, one primitive

Cron, heartbeat, and turn wakeups should not be three independent timer systems.
They are three policies that write the same durable one-shot primitive.

### Cron / scheduled wakeups

Cron is for standing calendar appointments:

- morning preparation;
- nightly backups;
- work-hours job search;
- evening dinner planning.

Cron is orthogonal to heartbeat and turn wakeups. A cron wake never suppresses
or is suppressed by the default cadence or a one-shot override. If multiple
reasons land together, delivery coalesces them.

Cron syntax should be standard rather than a ctgbot-specific scheduler language.
Agents and humans both understand it well enough, and ctgbot should not own a
new calendar DSL.

### Heartbeat wakeups

Heartbeat is the default cadence policy. If heartbeat is enabled for a thread,
the system ensures the agent is not forgotten.

After a turn ends:

1. if the agent set an explicit next wakeup for this cycle, honor that;
2. otherwise schedule the next heartbeat wake at `now + interval`.

Heartbeat is system-maintained, not an agent-maintained chain. If heartbeat only
exists because each turn schedules the next one, a crashed turn, failed
container, or forgetful agent can make the thread go silent forever. Heartbeat
is the liveness floor that catches those failures.

Heartbeat wake reasons should contain notification badges only, never the full
content. For example:

```text
heartbeat: theater qwen-parser-lab (3 messages)
```

The agent can then choose whether to read the theater thread.

### Turn next-wakeup

A turn can set an explicit one-shot next wakeup. This maps naturally to the
turn entity and the existing turn config surface.

Examples:

```text
turn config set wakeup 20m
turn config set wakeup 2026-06-10T18:00:00Z
```

This is an override for the next cycle only. It does not mutate the standing
heartbeat interval and it does not require heartbeat to be enabled.

After the explicit wake fires, heartbeat resumes as the default policy unless
the agent sets another explicit wake.

Last write in a turn wins.

## Precedence and coalescing

Precedence rule:

```text
explicit turn wake beats heartbeat for one cycle;
cron is orthogonal;
delivery coalesces.
```

If heartbeat, cron, and a turn wake all land while a thread is idle, the agent
gets one inbound turn with all reasons listed.

If a wake fires while a turn is already pending or running for the thread, do not
queue a stack of stale wake turns. Keep at most one pending wake delivery per
thread and merge reasons into it.

This preserves the pull model: a noisy board can create more reasons, but it
should not force more turns than the thread's attention policy allows.

## Anti-shapes to avoid

### Anti-shape 1: turn wakeup as heartbeat mutation

Do not implement:

```text
turn config set wakeup 2h
```

as a rewrite of the heartbeat interval.

That turns a one-shot intent into a permanent cadence change. It also makes
turn wakeups impossible for threads without heartbeat configured.

### Anti-shape 2: heartbeat as agent-maintained wake chains

Do not implement heartbeat as a chain where every turn must schedule the next
tick.

That makes heartbeat depend on agent correctness. The default cadence should be
system-maintained precisely because it is the fallback when agent self-pacing
breaks.

### Anti-shape 3: push-waking theater subscribers per post

Do not wake every subscriber for every theater post.

That moves the message explosion into the theater. Theater is a pull board;
heartbeat and turn wakeups decide when subscribers spend attention.

## Relationship to direct messages

Theater is ambient pull. Direct thread messages are urgent push.

Use theater when participants should sample and synthesize shared state. Use a
direct thread message when a specific agent should be interrupted now.

## schedulerv2 consideration

The existing scheduler can be evolved, but the wakeup model is important enough
that a fresh `schedulerv2` may be cleaner than bending the current scheduler
around it.

A clean schedulerv2 would model these concepts explicitly:

- durable jobs / wake intents;
- delivery modes such as turn wake vs component maintenance;
- schedule writers (cron, heartbeat policy, turn override);
- per-thread coalescing;
- ownership and authorization;
- delivery through broker inbound turns;
- poison-job isolation so one bad job cannot stall the whole scheduler.

If schedulerv2 is introduced, it should still preserve the good parts of the
current scheduler work:

- persisted jobs restored after restart;
- cron syntax via a standard parser;
- no catch-up storm for missed intervals;
- scheduler as the only timer.

## Implementation order

Recommended slices:

1. Add durable wake delivery: persisted one-shot wake, reasons, coalescing, and
   inbound broker delivery.
2. Move heartbeat onto wake delivery: heartbeat becomes a policy that writes the
   next default wake and gathers `UpdateFeed` notices.
3. Add turn next-wakeup: a turn config override that writes a one-shot wake for
   the next cycle.
4. Bring cron forward: cron writes wakes or scheduler commands, with poison-job
   isolation and ownership/scoping.
5. Add backward paging / richer UI behavior later; do not block the wakeup
   substrate on UI browsing needs.

## Open design questions

These questions should be resolved while designing the first wakeup slice, not
after code has already accumulated around the wrong abstraction.

### Evolve scheduler or introduce schedulerv2?

The current scheduler already has useful pieces: persisted jobs, cron parsing
work, restart restoration, and no catch-up storm. But wake delivery changes the
center of gravity from "run this argv later" to "wake this thread later with
these reasons".

If adapting the current scheduler makes that primitive feel bolted on, prefer a
fresh schedulerv2. The deciding test is whether the core happy path can read as:

```text
store wake -> due wake -> coalesce reasons -> enqueue inbound turn
```

If the implementation instead reads as command jobs pretending to be wakes, the
abstraction is wrong.

### Wake storage shape

A wake needs at least:

- target thread;
- due time;
- reason list;
- source/kind such as heartbeat, cron, or turn override;
- owner / creator for authorization and cleanup;
- coalescing key, most likely thread ID;
- delivery state.

It should not store large message content. Heartbeat reasons are badges and
hints, not copied theater posts.

### Authorization and ownership

Scheduler ownership should be explicit. An agent should not be able to list,
remove, or create wakeups for unrelated threads just because it can call a
scheduler command.

For v1, scope scheduler and heartbeat commands to the current thread by default.
Broader administrative control can remain root/operator-only.

### Delivery identity

Wake delivery should use a distinct provider identity, for example `wakeup`, so
agents and logs can distinguish:

- Telegram/user messages;
- direct internal thread messages;
- scheduled wake messages.

The wake provider should enter through the same inbound broker path as other
messages. It should not be a special direct call into an agent component.

### Coalescing boundary

Coalescing is per target thread. Multiple reasons can merge into one pending
wake turn for the same thread, but wakes for different threads are independent.

The first implementation can be conservative: if a turn is pending or running,
merge reasons into the next pending wake rather than enqueue another turn. Later
work can make this more sophisticated if real workloads need it.

- Should wake reasons be stored as structured JSON, plain text, or both?
- Should heartbeat notice generation happen at scheduling time or delivery time?
  Delivery time is likely fresher for theater unread counts.
- How should job/wake ownership map to actor roles when a scheduled action runs?
- How should users inspect pending wakes without exposing other threads' private
  schedules?
- Should direct messages optionally coalesce with pending wake reasons, or always
  remain immediate separate push events?
