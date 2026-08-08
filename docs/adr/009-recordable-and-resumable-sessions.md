ADR 009: Sessions are recorded whole and resumable from any turn
Status: Accepted
Supersedes: the session directory's naming and creation rules in ADR 007

Context
- Yesterday's secret-keeping work spent about 130 live runs to move one number.
  Every run replayed the same two character-creation turns before reaching the
  turn under test, and every run judged the DM by grepping a terminal capture.
- The terminal capture is the wrong artefact twice over. It is hard-wrapped to
  whatever width the terminal had, so `secrets-check.sh` has to flatten newlines
  before a phrase will match; and under `-debug` it is interleaved with request
  lines, so a run cannot be both greppable and inspectable.
- The request log already writes everything sent and everything returned, one
  file per call. It is held in memory until the Session directory exists — and
  the directory is created from the first World update block, which about a
  third of runs never emit. Those runs lose the whole log, which is exactly the
  set of runs worth reading.
- Nothing records which setting and adventure a session was played in. The
  harness knows at startup and forgets.

Decision
- The Session directory is created at startup and numbered:
  `saves/NNNN-<adventure>/`. The character's name is no longer part of it. The
  name was only available from the first block, and making the directory wait
  for it is what loses the logs.
- `metadata.json` is written into it at startup: setting, adventure, language,
  model, start time, and — when the run was resumed — the sidecar it started
  from. It is the only structured file the harness writes for itself, and it is
  what makes a recorded session self-describing.
- Under `-debug`, every request is logged as a pair: `log/context-NNN.md`, the
  readable record of everything sent and returned, and `log/context-NNN.json`,
  the exact `[]llm.Message` that went on the wire. Numbering is per request and
  zero-based, with the turn in the header — a turn makes several requests and
  collapsing them would hide the tool rounds.
- `transcript.md` is written every turn, unconditionally: the prose the player
  read and the lines they typed, in order, nothing else. Unwrapped, so a phrase
  survives as a phrase. This is what a test suite greps.
- `-resume <context-NNN.json>` starts a session at that turn. No prior turn is
  played: the recorded messages are loaded into the history as they are, and the
  first call the run makes is the turn under test. A model holds no state, so a
  turn is entirely defined by the context handed to it, and a recording that is
  faithful can be injected at any point. The recorded system prompt and briefing
  are discarded and rebuilt from today's prompt files; the conversation is taken
  from the file unchanged. Editing a prompt and re-running the same context is
  then a measurement of that edit.
  - Setting and adventure come from the recorded session's `metadata.json`,
    found by walking up from the context file.
  - The briefing rebuilt is the plain adventure briefing, not the opening one:
    the opening briefing asks for a character to be made, and the resumed run
    already has one. This is the same substitution a Milestone makes, for the
    same reason.
  - The trailing message — the player's line for that turn, with the Charsheet
    folded in — is dropped, and that line comes from the script instead. So
    `context-006.json` costs one call and produces turn 6.
  - The Charsheet to start from is read from the recorded session's
    `charsheet-log.md`, the last block written at or before that turn, rather
    than cut back out of the fold. Cutting would mean matching the charsheet
    header text as it was when recorded, and that text is one of the things
    under test.
  - A resumed run gets its own Session directory. The recording is never written
    into.

Consequences
- Any turn of a recorded run can be re-run for exactly one call instead of N.
  The two character-creation turns, about a third of every call the leak suite
  makes, stop being paid for entirely.
- The missing-block note stays gated on a sheet having been written at least
  once, not on the directory existing — the directory is no longer evidence of
  anything. The hole this leaves is deliberate and recorded in `AGENTS.md`: a
  session whose opening turn emits no block is still never corrected. What
  changes is that its logs now survive, so it can at least be read.
- `saves/` is no longer browsable by character name. `metadata.json` and
  `transcript.md` are what identify a run now, and both are greppable.
- Nothing parses the Charsheet any more. The name regex that gave the directory
  its name is gone, and with it the last piece of code that read the sheet.
- A resumed run continues from a conversation the current prompts did not
  produce. The model will happily carry on from prose that today's rules would
  have forbidden, so a resumed run measures the prompt's effect on *this* turn,
  not on the run that led to it. Whole runs still have to be played for anything
  else.
- The recording is only as good as its worst gap. A sidecar holds the messages
  sent for that request, so a turn's reply first appears in the *next* request's
  file — which means the last reply of a session is in no sidecar at all.
  `log/context-final.json` is written when the session ends to close that gap.
