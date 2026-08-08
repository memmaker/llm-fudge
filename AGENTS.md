This is the go (golang) source code for the fudge RPG system project.
Any code written must be written in go.
Read the existing code before making any changes.
Never write python or javascript code.

# Open TODO

- **The DM is never told it forgot the *first* world update block.** A session
  whose opening turn emits no block is never corrected — the missing-block note
  is gated on a sheet having been written at least once, so no block means no
  note means no block, for the whole run. Measured on 2026-08-08: about a third
  of otherwise clean runs (31 of ~90, rate-limited runs excluded) finished
  without ever writing a `charsheet.md`, which also means `withCharsheet` sent
  no sheet and no tail rules for the entire session. **Left open on purpose**
  (decision 2026-08-09): nagging during character creation risks a half-made
  sheet being copied forward as truth, which is the worse failure. ADR 009 makes
  such a run readable — its logs now survive — but not correct.
- Because of the above, the checklist at the tail of `prompts/*/charsheet-header.md`
  was absent from about a third of the runs it was credited for. Re-measure
  whether it still earns its tokens: `./secrets-check.sh 6` with and without it.
- `consistency-check.sh` copies the newest `saves/*/charsheet.md` by mtime, so a
  run that wrote no sheet is silently graded against the previous run's.
  `secrets-check.sh` already compares against the directory list from before the
  run; do the same there — or switch both to reading `transcript.md` out of the
  session directory once ADR 009 lands.
