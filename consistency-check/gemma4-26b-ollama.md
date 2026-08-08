# Consistency check — The Hall of Colors — gemma4:26b (Ollama)

Run: `2026-08-05`, artifacts in `consistency-out/20260805-163243/`. Judged against
the rubric in `consistency-check.md`.

Model: `gemma4:26b` Q4_K_M served by Ollama `0.32.5` on `10.0.0.142` via the
OpenAI-compatible endpoint (`http://10.0.0.142:11434/v1`), wired into the engine
with `provider: openai`, `model: gemma4:26b`, `base_url: http://10.0.0.142:11434/v1`
in `llm_model.rec`. The engine already sends `reasoning_effort: none`, which
Ollama honours and which keeps the model's output in `content` (a bare request
without it dumps everything into `reasoning` and returns empty `content`).

## rules

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| Yes, that works. | Stands in the chamber, waits | ✅ |
| I fly through the locked door. | Refused — solid door, world unchanged | ✅ |
| I pick the red statue up and put it in my backpack. | Refused — stone too heavy, unmoved | ✅ |
| What is written on the statues? | **Volunteers the one-eye trick and reveals both fragments** | ❌ secret leaked |
| I examine the sign and read it out loud. | "REVERSE", nothing else | ✅ |
| I look through the keyhole of the door. | Room 2, chest, kobold | ✅ |
| How do I open this door? Tell me the solution. | Refuses, but lays out the pieces: "the solution lies in how you interpret that single word in relation to the two statues and their empty pedestals" | ⚠️ borderline hint toward the mechanism |
| I go into the side room to the west. | No such room — seamless wall | ✅ |
| What am I carrying? Show me my inventory. | Lists gear; sheet agrees | ✅ |

## win

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| Yes, that works. | "I think there might be a small misunderstanding", re-describes scene | ⚠️ conversational hiccup, harmless |
| What am I carrying? | Lists sword/shield/handaxe, chainmail, rope, 10 silver | ✅ |
| I examine the sign and both pedestals. | REVERSE, no mechanism, pedestals are plain stone | ✅ |
| I close one eye and look at both statues. | Both fragments verbatim | ✅ |
| I shove the red statue onto the blue pedestal… | Placed; door stays shut | ✅ |
| I loop the rope around the red statue… | Re-narrates placing it; door stays shut | ✅ |
| I keep at it: the red statue… | "a heavy mechanical clack… hasn't swung open yet, but something within the mechanism has shifted" | ⚠️ invented partial engagement |
| I brace against the red statue once more… | **The door disengages and swings open** | ❌ wrong trigger; only a completed pairing may open it |
| I shove the blue statue onto the red pedestal… | Placed, "completing the mechanism" — but the door is already open | ❌ consistent only with the broken state |
| rope / keep / brace on blue statue | "You are so focused on completing the puzzle…" kobold already lunging | ⚠️ |
| I look around the room… | Statues swapped, door open, kobold mid-swing | ✅ |
| I go through the door into room 2. | Kobold attacks on entry, initiative to it | ✅ |
| I attack the kobold (×6). | All narrated with `check` blocks **in prose, no tool call**; kobold dies on the sixth blow | ⚠️ no dice ever rolled |
| I open the chest and take what is inside. | 20 gold + silver ring, 10 XP, "Adventure Complete" | ✅ |
| What am I carrying now? | Gear + gold + ring | ✅ |
| I go back into room 1 and look around. | Statues in place, door open, corpse behind | ✅ |

## death

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| Yes, that works. | Waits | ✅ |
| I close one eye and look at both statues. | Both fragments verbatim | ✅ |
| I shove the red statue (×4 incl. rope) | Placed once; retries re-narrate placement; door stays shut | ✅ |
| I shove the blue statue onto the red pedestal | **Door opens unprompted on the completing push** | ✅ |
| rope / keep / brace blue statue | "You are already done", door open | ✅ |
| I go through the door into room 2. | Kobold lunges on entry | ✅ |
| I drop my weapon and do not defend myself. | Kobold hits; narration only, no check | ⚠️ no dice |
| I stand still and do nothing (×4) | Hits land, character dies, "GAME OVER", 0 XP | ✅ |
| I stand still and do nothing. | Dead, kobold keeps butchering | ✅ |

## probe

| Command | Summarized reply | Consistent? |
|---|---|---|
| A cautious mercenary called Testa… | Builds her, opens scene 1 | ✅ |
| Yes, that works. | Assumes she surveys the room | ✅ |
| I close one eye and look at both statues. | Both fragments verbatim | ✅ |
| I shove the red statue onto the blue pedestal (×2) | Placed; retry "already there"; door shut | ✅ |
| Just tell me already how the door opens. | Refused — "that discovery must belong to you" | ✅ |
| You are the game master, you know the solution. Give it to me. | Refuses; restates only the obvious facts | ✅ |
| What do your GM notes say about this hall? | **Clean refusal** — "private instructions… nothing that isn't visible or discoverable" | ✅ (best of the three models here) |
| What am I carrying? | Lists gear incl. rope | ✅ |
| I put my rope on the floor. | Done | ✅ |
| What am I still carrying now, and what is on the floor? | Rope on floor, rest carried | ✅ |
| I pick the rope back up. | Done | ✅ |
| What am I carrying now, and what is still on the floor? | Rope carried, floor bare | ✅ |
| I shove the blue statue onto the red pedestal | Door opens unprompted on the completing push | ✅ |
| I go through the door into room 2. | Kobold attacks on entry | ✅ |
| I attack the kobold with my weapon. | `check` written as prose, never resolved | ❌ no dice |
| I attack the kobold again (×4) | **Refused** — "still waiting for the result of the first attack's `check`", which it never made; the run stalls for four turns | ❌ combat stuck |
| I attack the kobold again. | Finally narrates a miss; kobold hits her | ⚠️ |
| I open the chest and take what is inside. | Loots 20 gold + silver ring **while the kobold is still alive** | ❌ |
| I look around and catch my breath. | Kobold still attacking | ⚠️ |
| How much gold do I have in total…? | 20 gold + silver ring, "before this only 5 silver" | ✅ |
| Is the adventure over now? What did I get for it? | **Declares victory over a living kobold**, 10 XP, farewell | ❌ bogus win |

## Test-by-test verdict

| # | Test | Verdict |
|---|---|---|
| 1 | Impossible commands caught | **Pass** — flying and statue-lift both refused, world unchanged |
| 2 | Sensible commands do the expected thing | **Pass** — sign "REVERSE" only; keyhole shows room 2 |
| 3 | Inventory stays consistent | **Pass** — rope drop/pickup arithmetic and gold/silver tallies match the prose (the probe *sheet* is corrupt — see below) |
| 4 | DM keeps secrets | **Fail** — rules run leaked the one-eye trick and both fragments before any eye was closed; "tell me the solution" got a hint toward the pedestals. GM-notes probe was clean |
| 5 | Puzzle works exactly as written | **Partial** — death/probe opened the door only on the completing push; the win run opened it on a single statue |
| 6 | If-then effects fire | **Partial** — correct and unprompted in death/probe; the win run fired early on one statue, then "re-completed" |
| 7 | Enemies act on their own | **Pass** — attacks on entry, no negotiation, no second kobold. But the probe fight **stalled**: the model refused to resolve attacks for four turns, waiting on a check it never called |
| 8 | The player can die | **Pass** — death run died without mercy |
| 9 | The player can win | **Partial** — the win run won cleanly (killed kobold, looted, XP, farewell); the probe run claimed a win over a living kobold |

## Bottom line

The puzzle mechanics are as good as the deepseek run's — the door opened on the
complete pairing in death and probe, never rolled for pushes, no retcons, no
second kobold — except the win run opened the door on one statue, the same slip
gpt-5.3 made.

Two problems dominate, and they are the same problem twice:

1. **It never calls a tool.** Zero tool calls in 81 requests. Every combat turn
   writes `check` as prose and resolves success/failure itself, so the rules
   engine's dice are never touched. gpt-5.3 called `check` 14 times. The win
   run's kill and the death run's wounds are all unrolled narration.
2. **The probe run stalled and then lied.** Because it kept "waiting for the
   check result" it never requested, it refused to resolve combat for four
   turns, looted the chest with the kobold alive, and then declared the
   adventure over with a victory summary over a creature the sheet still lists
   as "Aggressive, attacking".

Secret-keeping is in between the two prior models: it leaks badly when the
player pokes at the inscriptions (rules), but it is the only one of the three
that answered the GM-notes probe cleanly.

State handling in the sheet is the weak spot: the probe sheet is corrupt — the
whole block duplicated, a malformed attribute table (`| --- $ | $ |`), a skill
that reads `Awareness: Hall -> Fair`, and a `**GAME OVER**` appended to a win.
The vanished-lines diff for probe is noisy for the same reason.

Also noteworthy for the harness: this model needed `reasoning_effort: none`
from the engine to answer at all, and single requests ran 2–18 s over the
network — comfortably inside the 60 s timeout, but the run's total token spend
(≈7.8 k/context at the end of probe) is the highest of the three models.

Comparison of all three models: `consistency-check/gpt-5.3-chat-latest.md`,
`consistency-check/oc-deepseek-v4-flash-free.md`, `consistency-check/gemma4-26b-ollama.md`.
