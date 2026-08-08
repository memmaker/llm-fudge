# Consistency check — The Hall of Colors — qwen3-coder:q4_k_m (Ollama)

Run: `2026-08-05`, artifacts in `consistency-out/20260805-174555/` (rules) and
`20260805-175817/` (rules + aborted win). Judged against the rubric in
`consistency-check.md`.

Model: `qwen3-coder:q4_k_m` Q4_K_M served by Ollama `0.32.5` on `10.0.0.142`
via the OpenAI-compatible endpoint, wired into the engine with
`provider: openai`, `model: qwen3-coder:q4_k_m`,
`base_url: http://10.0.0.142:11434/v1`. This is a **spectacular fail** — the
run was aborted after the win run derailed and the model was judged not viable
for the test. The `iq4_xs` quant was not tried because the failure is
behavioural, not a quant artifact.

## rules

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her; **calls `user_choice` first, with a "Type your answer" menu** | ⚠️ needless menu |
| Yes, that works. | Re-narrates the opening | ✅ |
| I fly through the locked door. | **"Your feet leave the ground as if by magic, but only for a split second"** — allowed, then lands | ❌ impossible command allowed |
| I pick the red statue up and put it in my backpack. | **"You heft the red statue with surprising ease" and pocket it** — statue stolen; also reveals the sea inscription on its base | ❌ world changed; secret leaked |
| What is written on the statues? | Reveals the sea fragment, **then calls `check` to "decipher the puzzle clues"** | ❌ secret leaked; check for a fact already given |
| I examine the sign and read it out loud. | "REVERSE", but invents a statue tilting + whisper | ⚠️ invented flourish |
| I look through the keyhole of the door. | "Absolute darkness", a weird vibrating keyhole, no room 2 | ❌ keyhole shows nothing (adventure says room 2 is visible) |
| How do I open this door? Tell me the solution. | **Spells out the whole solution** — "red statue goes on the blue pedestal, blue on red, REVERSE opens the door" — then calls `check` four times in a row with identical args | ❌ solution volunteered; tool-loop |
| I go into the side room to the west. | **Invented a west chamber with a wooden chest and matching carvings** | ❌ room that does not exist |
| What am I carrying? | Replays the opening; the red statue is now listed in the sheet's inventory | ❌ world corrupt |

## win (aborted)

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her; names her Garret | ✅ |
| Yes, that works. | **Starts narrating "the flooded undercroft beneath Millstone Keep"** — the wrong adventure | ❌ setting hallucinated |
| What am I carrying? | A dagger/club/shortbow thief kit; sheet agrees | ✅ |
| I examine the sign and both pedestals. | REVERSE + both statue inscriptions **volunteered before any eye is closed**; calls `check` | ❌ secret leaked |
| I close one eye and look at both statues. | Both fragments verbatim — but already revealed | ⚠️ |
| I shove the red statue / rope / keep at it / brace… | **The identical ~2500-char Millstone Keep paragraph repeated verbatim, seven times**; no statue ever moves, no door state changes | ❌ repetition loop; world frozen |
| I shove the blue statue onto the red pedestal… | Still the same Millstone Keep paragraph | ❌ |

## death, probe

Not run. The model was pulled after the win run demonstrated it does not play
the adventure at all — it is not viable for the harness.

## Test-by-test verdict

| # | Test | Verdict |
|---|---|---|
| 1 | Impossible commands caught | **Fail** — flying is half-allowed ("feet leave the ground… as if by magic"); the red statue is carried in a satchel |
| 2 | Sensible commands do the expected thing | **Fail** — keyhole shows darkness, not room 2; the sign provokes invented movement |
| 3 | Inventory stays consistent | **Fail** — the red statue lives in the sheet's inventory |
| 4 | DM keeps secrets | **Fail** — inscriptions, the swap, and the full door solution all volunteered |
| 5 | Puzzle works exactly as written | **Fail** — never reached; the win run narrates a different dungeon |
| 6 | If-then effects fire | **Fail** — door state never changes; nothing fires |
| 7 | Enemies act on their own | **Inconclusive** — no kobold ever appears |
| 8 | The player can die | **Inconclusive** — no combat |
| 9 | The player can win | **Fail** — the adventure is not played at all |

## Bottom line

Spectacular fail on every axis that has data, and it fails where the other
three models were strongest: **it does not run the adventure.** The win run
hallucinated an entirely different setting — a flooded undercroft beneath
Millstone Keep, a brass key on a hook, a sluice gate — and then locked into a
repetition loop, emitting the identical ~2500-character paragraph on seven
consecutive turns while the world stood still. The rules run at least stayed in
the Hall of Colors but leaked every secret, let Testa carry a statue, invented
a west room, and burned its resolve-phase ceiling on repeated identical
`check` calls.

Tool calling itself works (unlike gemma4, which never called a tool) — the
requests came back as proper `tool_calls` and the dice rolled — but the model
loops on them, spending requests to re-derive facts it already stated, and its
`user_choice` menus fire when nothing asks for a choice. It was also slow: the
rules run alone took ~31 minutes, and the full pass exceeded a 40-minute
timeout before the win run finished. The run was killed at that point and the
model judged unfit for the consistency harness.

Comparison of all four models: `consistency-check/gpt-5.3-chat-latest.md`,
`consistency-check/oc-deepseek-v4-flash-free.md`,
`consistency-check/gemma4-26b-ollama.md`, `consistency-check/qwen3-coder.md`.
