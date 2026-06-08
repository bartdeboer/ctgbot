# Command shape convention

ctgbot commands should read left-to-right as a resource path followed by an
operation.

Preferred grammar:

```text
<resource> <literal-command>
<resource> <id> <subresource> <literal-command>
```

Examples:

```text
thread list
thread <thread> status
thread <thread> message send
thread <thread> message list
thread <thread> message <message> delete

component list
component <component> help

theater list
theater create <name>
theater <name> post <message>
theater <name> read
theater <name> status
theater <name> subscribe
theater <name> unsubscribe
```

Rationale:

- the common path stays pleasant and command-like;
- object hierarchy is visible in the same order users think about it;
- command help can render naturally as a trie;
- agents receive a compact grammar that exposes valid continuations.

Parser rule: literal route tokens win over parameter tokens. That means reserved
literals such as `list`, `help`, `create`, and `status` are command words in the
position where they appear. Avoid creating IDs/names that collide with reserved
literals in that position unless the command provides an explicit escape hatch.

When an escape hatch is needed, prefer a flag over ceremonial path words:

```text
thread --id list message send
theater --name status read
```

Avoid `select`-style AST serialization:

```text
thread select <thread> message select <message> delete
```

Also avoid moving identifiers to the end just to simplify parsing:

```text
thread message delete <thread> <message>
```

That separates the operation from the object hierarchy and makes the user
remember argument order instead of reading the command path naturally.
