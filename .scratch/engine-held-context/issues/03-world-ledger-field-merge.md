# 03 — World ledger with field-level merge

**What to build:** A ledger holding the fiction facts the rules library cannot compute — dispositions, positions, scene details, open threads — as one entry per entity, merged as the DM reports changes, and rendered to dense text the DM reads.

The grammar, which both the parser and the DM prompt must agree on (settled during design; it encodes the decision more precisely than prose):

```
entry  := key ("|" token)*
token  := "k:v"     replace field k on the entry, keep every other field
        | "flag"    add flag to the entry if absent
        | "!name"   remove field or flag `name`
        | "GONE"    drop the entry entirely (used as key|GONE)
```

Field merge, not entity replacement: merge fails by leaving a visible stale token, replacement fails by silently deleting facts the DM did not restate. The DM writes partial lines today and will keep doing so.

**Engine-owned fields are stripped on ingest.** When an entry's key matches a Character id, any field the rules library owns — wound level, scratches, Fudge Points, coins, carried Items, attributes, skills, gifts, faults, scale, species, id — is discarded before merging, and the discard is surfaced in debug output. This makes overlap between the roster and the ledger impossible by construction rather than by instruction; measured sessions show the DM reporting wounds it does not own.

Each entry records the Turn it was last set and renders it as a marker, so a fact can be traced to its source Turn.

Context: `docs/engine-held-context-spec.md`, WORLD ledger. This is the feature's new test seam.

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] A `k:v` token replaces only that key and leaves every other field on the entry intact
- [ ] A bare flag is added once; reporting it again does not duplicate it
- [ ] `!name` removes a field or a flag
- [ ] `key|GONE` drops the whole entry
- [ ] Engine-owned fields are stripped from entries keyed to a Character id, and the strip is reported in debug output
- [ ] Entries whose key is not a Character id keep every field, including ones that share a name with an engine-owned field
- [ ] Each entry renders a marker for the Turn it was last set
- [ ] A malformed line preserves its fact rather than dropping it, and does not corrupt other entries
- [ ] Rendering is deterministic across runs
- [ ] Table-driven tests assert on rendered output, never on internal representation
