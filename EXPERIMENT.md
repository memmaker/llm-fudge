Branch `charsheet-template-blank`, off `world-update-block` — everything that
branch's note says still holds; this one changes character creation on top of it.

Arm 1, measured and reverted: the sheet template's Equipment section kept its
headings and lost its example values. Invention did not move, 5 of 5 either way
(`consistency-out/20260809-161240-repro-invented-content` against
`20260809-162359`), and the sheet lost the units the example carried — `50 ft`
and `x3` for `Rope (15 m)` and `Torches ×3`. So the example is not what is being
copied, and the template is back.

Arm 2, in place: `charsheet-none.md`, the only prompt sent while no sheet exists,
says the example's gear is not the character's — they carry what the player
named and nothing else, and picking their traits means attributes, skills, gifts
and faults, never equipment.

What it is trying to settle: creation put gear on the sheet the player never
asked for in 5 of 5 runs — armour, a second weapon, silver. The player's own
line is what invites it ("pick the rest of her traits yourself"), and nothing in
the prompts says traits stop short of the pack.
