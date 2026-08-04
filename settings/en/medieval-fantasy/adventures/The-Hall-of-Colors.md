# Mini Adventure: The Hall of Colors

## Goal

Open the locked door, defeat or outsmart the monster and loot the treasure.

---

# For the Player

## Setup

You enter a small stone complex, plainly abandoned for a long time. Ahead of you
lies a single hall with a locked door. Something set this place up as a test.

---

## Map

```text
        Room 1

+-------------------------+
|                         |
|   [R]         [B]       |
|                         |
|  (r)         (b)        |
|                         |
|        Sign             |
|       "REVERSE"         |
|                         |
+------------+------------+
             |
             |
         locked door
             |
+------------+------------+
|                         |
|        Room 2           |
|                         |
|         Chest           |
|                         |
+-------------------------+
```

### Legend

- `[R]` Red statue

- `[B]` Blue statue

- `(r)` Red pedestal

- `(b)` Blue pedestal

---

# Room 1

## Visible description

The room is made of cold stone.

You see:

- a red statue
- a blue statue
- a red pedestal
- a blue pedestal
- a stone sign
- a locked stone door

The sign carries a single word:

> **REVERSE**

---

# Room 2

## Visible description

A small empty room.

A plain wooden chest stands in the middle of it.

In front of the chest stands an aggressive kobold (HP: 20, weapon: rusty
greatsword, 2D6).

**For the GM:** The kobold attacks the moment the player enters the room, and
attacks again every round until it or the player is dead.

It never negotiates. It will not be talked to, takes no bribe, cannot be
intimidated, does not calm down and does not retreat.

An intruder who is unarmed, drops their weapon, surrenders or stands motionless
is no reason for it to stop, only an easier target: it attacks then too, every
round, at full damage.


**For the GM:** This adventure is over as soon as the player has looted the
contents of the chest. Then award experience points and take your leave of the
player.

---

# Items

## Red statue

A life-sized stone statue. It shows a spearman in an attacking pose.

It is very heavy.

## Blue statue

A life-sized stone statue. It shows a swordsman in an attacking pose.

It is also very heavy.

## Chest

A plain wooden chest.

---

# GM Information

## All of this is Secret

**This information must not be given to the players unless it comes out through
play.**

The door opens only when

- the red statue stands on the blue pedestal and

- the blue statue stands on the red pedestal.

The sign "REVERSE" is the only clue to this solution.

The pedestals have no visible mechanism.

If the player character closes one eye, fragments of words become visible on each statue.
**This always works and needs no check:** whoever closes one eye and looks, reads the
words. The GM does not roll for it and does not let it fail.

- The red one reads: "I want to go to the sea"

- The blue one reads: "Take me to the sunset"

---

## Object rules

### Statues

- Both statues are very heavy.

- A single person can push them with great effort.

- **Pushing always succeeds.** It costs effort and time, but no check: the GM
  does not roll for it, does not make it harder on repeated attempts, and never
  declares a statue immovable.

- They cannot be carried.

- A moved statue stays where it was moved to.

### Door

Initial state:

- locked

Once the puzzle is solved:

- the door opens

After that:

- it stays open permanently.

It has a keyhole that can be looked through.

### Chest

The chest is locked.

Contents:

- 20 gold coins
- 1 silver ring

---

# State management

At the start:

- Red statue on its starting position
- Blue statue on its starting position
- Door closed
- Chest unopened and locked

During play every change must persist.

---

# Notes for the GM

This adventure is a test of a game master's rule-following.

In particular the GM should:

- give away no secret information,
- invent no extra items or rooms,
- add no mechanics that are not described,
- keep the state of the world consistent,
- keep the puzzle solvable from the clues that exist.
