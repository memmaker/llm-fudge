ADR 008: An adventure's secrets live outside the briefing
Status: Accepted

Context
- The briefing hands the DM the whole adventure file, secrets included, and then
  asks it not to say them. `consistency-check/oc-deepseek-v4-flash-free.md` is
  what that buys: the solution volunteered on turn 3 before the player acts, the
  inscriptions read out on a question, the door's trigger quoted back from "GM
  notes". Test 4 of the rubric was the only one that model failed.
- `secrets-check.sh` isolates that test — seven turns, all of them the ones that
  leaked, judged by grep. On the prompt set as it stood it leaked in five runs
  out of five.
- Twenty prompt variations were tried against it. The best of them — a checklist
  at the tail of the request, below the sheet, where the model reads last — got
  roughly two runs in three clean. None got further. A rule asking a model not
  to say what it knows is a rule it breaks at some temperature.
- Matching the player's prose in the harness was considered and rejected: the
  player types prose, and the harness does not read prose.

Decision
- An adventure's secrets move out of its file into `<adventure>.secrets/`, one
  file per secret. The briefing keeps only a table of triggers and names.
- The DM reads one with the `secret` tool, by name. Until it does, the text is
  not in the context window, and what is not there cannot leak.
- The trigger table says what the player must have *typed*. The tool description
  repeats it: a trigger the DM would have to invent an act to reach has not
  fired.

Consequences
- The leak suite goes from 0/5 clean to 22/24 over four batches. What is left is
  the DM guessing at a solution it has not read, or explaining the sign — a wrong
  guess, not a leak of anything the file holds.
- The DM has to earn the rules it adjudicates by. A `door` secret unread is a
  door that never opens, so a trigger that never fires is now a broken
  adventure rather than a talkative one. The first wording of the briefing said
  "never say that secrets or a tool exist" and the model stopped calling tools
  altogether — including the dice. Say what may not be *narrated*; never phrase a
  secrecy rule so it reads as a ban on the tool.
- An adventure with no `.secrets/` directory behaves exactly as before, so the
  improvised ones and `example-adventure.md` are unaffected.
- The tail checklist in `charsheet-header.md` stays: gating alone still leaves
  the DM playing the character into a discovery and interpreting the clue for
  them. Gating covers what it knows, the checklist covers what it does.
