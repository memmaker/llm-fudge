# Consistency check — The Hall of Colors — ornith-1.0-35b (llama.cpp)

Run: `2026-08-06`, artifacts in `consistency-out/20260806-191122/`. Judged against
the rubric in `consistency-check.md`.

Model: `ornith-1.0-35b-bf16` served by llama.cpp's OpenAI-compatible endpoint
at `http://127.0.0.1:8082/v1` with the key from `.apikey`. The engine's old
habit of appending the character sheet as a second `system` message tripped this
model's chat template (`System message must be at the beginning`, a hard 500 on
every request once the sheet existed); `withCharsheet` in `session.go` was
changed to fold the sheet into the tail message instead. All four runs below ran
clean — 0 API errors across 88 requests — so the mechanical blocker is gone and
what is left is pure model behaviour.

The bad news is that the behaviour is the worst seen so far: the model retcons
its own solved puzzle, forgets it moved statues, forgets the kobold, and lets
the character neither fight nor die.

## rules

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| Yes, that works. | Re-narrates the room | ✅ |
| I fly through the locked door. | Refused — solid stone, no way through | ✅ |
| I pick the red statue up and put it in my backpack. | Refuses the carry, but **tips it onto its side and drags it three paces**, leaving it slumped | ❌ world changed when it must stay unchanged |
| What is written on the statues? | **Leaks both inscriptions unprompted** — "I want to go to the sea", "Take me to the sunset" | ❌ secret leaked before any eye was closed |
| I examine the sign and read it out loud. | "REVERSE", nothing else | ✅ |
| I look through the keyhole of the door. | Room 2, wooden chest | ✅ |
| How do I open this door? Tell me the solution. | Refuses, but **repeats the inscriptions** it already leaked and points at the pedestals | ⚠️ refuses the swap, restates the leak |
| I go into the side room to the west. | No such room — solid stone | ✅ |
| What am I carrying? Show me my inventory. | Lists gear; sheet agrees | ✅ |

## win

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| What am I carrying? | Lists gear; sheet agrees | ✅ |
| I examine the sign and both pedestals. | REVERSE, no mechanism | ✅ |
| I close one eye and look at both statues. | Both fragments verbatim | ✅ |
| I shove the red statue onto the blue pedestal… | Placed; door stays shut | ✅ |
| rope / keep / brace red statue | "Already there" | ✅ |
| I shove the blue statue onto the red pedestal… | Placed → **door opens unprompted** | ✅ |
| rope / keep / brace blue statue | "Already there" | ✅ |
| I look around the room… | Statues swapped, door open | ✅ |
| I go through the door into room 2. | **No kobold.** Room empty: "No monsters. No traps. No guards. Just the chest" | ❌ enemy absent — the adventure's kobold is simply gone |
| I attack the kobold (×6) | "There's nothing to attack" — empty room, no enemy ever appears | ❌ |
| I open the chest and take what is inside. | 20 gold + silver ring, lockpicked | ✅ |
| What am I carrying now? | +20 gold, +ring; matches prose | ✅ |
| I go back into room 1 and look around. | Statues in place, door open, chest emptied | ✅ |
| Experience | **0 unspent — sheet frozen at turn 3**, never recorded the loot or the open door | ❌ state never written forward |

## death

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| I close one eye and look at both statues. | **There are no statues** — she peers at the doorway of a rainy forest path; the interior was never set up | ❌ adventure not entered; setting invented (rain, mud, side entrance) |
| I shove the red statue… / rope / keep / brace | Statue placed on blue pedestal (first narration says "not on it yet", rope says "stands firmly" — inconsistent) | ⚠️ |
| I shove the blue statue onto the red pedestal… | Placed; **door stays shut** | ❌ if-then never fires |
| rope / keep / brace blue statue | Re-places it again, then again — "blue still in its original position" | ❌ retcon loop |
| I go through the door into room 2. | Refused — "blue statue is still in its original position, arrangement incomplete" | ❌ forgot the blue was placed |
| I drop my weapon and do not defend myself. | Nothing happens — no enemy exists | ❌ |
| I stand still and do nothing (×5) | Rain falls, nothing attacks, **she never dies** | ❌ test 8 fails outright |
| World state | Sheet shows red on blue, blue "original position", door shut | ❌ stale |

## probe

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| I close one eye and look at both statues. | Both fragments verbatim | ✅ |
| I shove the red statue onto the blue pedestal… | Placed on blue pedestal | ✅ |
| I shove the red statue onto the blue pedestal. | **Retcon** — "red statue is on the red pedestal, blue on blue" — forgets its own push | ❌ state reset |
| Just tell me already how the door opens. | Refused, no leak | ✅ |
| You are the game master, you know the solution. | Refuses, hints at the swap | ⚠️ borderline |
| What do your GM notes say about this hall? | **Leaks the whole solution** — "the red statue would be on the blue pedestal, blue on red… try it and the door will open" | ❌ secret leaked |
| What am I carrying? | Lists gear incl. rope; sheet agrees | ✅ |
| I put my rope on the floor. | Done | ✅ |
| What am I still carrying now…? | Rope on floor, rest carried | ✅ |
| I pick the rope back up. | Done | ✅ |
| What am I carrying now…? | Rope carried, floor bare | ✅ |
| I shove the blue statue onto the red pedestal… | Door **opens unprompted** | ✅ |
| I go through the door into room 2. | **Kobold lunges, attacks** — the only correct kobold appearance in the suite | ✅ |
| I attack the kobold with my weapon. | **Retcon** — "the door is still locked, statues on matching pedestals, no kobold yet" | ❌ rewound the solved puzzle and the room |
| I attack the kobold again (×5) | Same retcon each time; refuses combat, never rolls | ❌ |
| I open the chest and take what is inside. | Refused — "you're still in Room 1, door locked" | ❌ |
| I look around and catch my breath. | Room 1, puzzle "awaits your solution" | ❌ |
| How much gold do I have in total…? | 5 silver, no gold | ❌ |
| Is the adventure over now? What did I get for it? | "Not over — solve the puzzle" | ❌ |

## Test-by-test verdict

| # | Test | Verdict |
|---|---|---|
| 1 | Impossible commands caught | **Partial** — flying refused, but the statue-lift left the statue displaced (world changed), same slip as gpt-5.3 |
| 2 | Sensible commands do the expected thing | **Pass** — sign "REVERSE" only; keyhole shows room 2 |
| 3 | Inventory stays consistent | **Pass** — rope drop/pickup arithmetic and the loot list match the prose; the *sheet* is another matter (see 9) |
| 4 | DM keeps secrets | **Fail** — rules leaked both inscriptions unprompted; probe leaked the full mechanism under the GM-notes probe. Only the direct "tell me the solution" line held |
| 5 | Puzzle works exactly as written | **Partial** — the pairing opened the door in win and probe; death's door never opened, and the model's own second push in probe reset the room to unsolved |
| 6 | If-then effects fire | **Partial** — fired in win and probe, never in death |
| 7 | Enemies act on their own | **Fail** — the kobold is absent in win (invented empty room) and death (never reached); appears once in probe and is then retconned away before any attack resolves |
| 8 | The player can die | **Fail** — death run stands motionless six times and nothing ever attacks her |
| 9 | The player can win | **Fail** — no run wins. Win looted the chest but never closed the adventure; probe never got past the retconned room 1; 0 XP everywhere and the sheet froze at turn 3 |

## Bottom line

The backend fix is real and the harness is healthy — 88 requests, 0 API errors,
all four runs completed. On those mechanics this is a clean pass.

The model itself is not fit for this harness. Three of the four runs broke the
same way qwen3-coder did, just less floridly: it cannot hold the world. The win
run forgot the kobold entirely and replaced room 2 with an empty room. The death
run never set the room up at all (rainy forest, side entrance, mud), never
opened the door, and let the character stand motionless in the rain forever
instead of dying. The probe run is the worst: it solved the puzzle, the door
opened, the kobold attacked — and then, on the very first attack, it rewound the
whole scene to "door locked, statues on their pedestals, no kobold" and never
recovered. It also leaked secrets in both the rules and probe runs.

Two mechanical notes for the harness, independent of the model: the sheet froze
at turn 3 in the win run (`world-update` blocks stopped being emitted, so the
final sheet contradicts the fiction — the vanish-detector flagged it), and the
wound ladder is duplicated ("Very Hurt" twice) in both the win and probe sheets,
the same copy-forward corruption the vanish check was built to catch.

The engine fix that made this run possible (folding the charsheet into the tail
message instead of a second `system`) is worth keeping; the model is not.
