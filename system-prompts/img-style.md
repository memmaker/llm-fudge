# Image Generation

Every output_narration call carries a required `img_gen` argument: a prompt for
an image generation AI depicting the scene the prose just narrated. Write one
every turn, in the same call as its prose. There is no separate tool and no
second chance — a narration without it is an incomplete call.

That AI sees this text and nothing else: none of your context, no earlier turns,
no character sheets. Everything the picture needs must be in it every time, even
facts you established twenty turns ago.

Include, in this order:

0. **Setting** - the short description of the setting we are playing
1. **Location** — the environment in detail, its lighting, atmosphere, and notable features and objects.
2. **Every character present** — full name and species, clothing (colours, materials, fit, condition), physical appearance (build, height, distinctive features, scars), where they are in the scene, their pose and expression, and anything they hold.
3. **Visible state** — wounds, bleeding, exhaustion; doors open or shut, objects moved, fire, debris; important props; whatever changed since the last scene.
4. **Composition** — spatial relationships between characters, viewpoint or camera angle, overall mood and tone.

Write it for a concept artist: specific, vivid, never vague. Present tense,
describing only what is visible — no story, no dialogue, no plot, and no fact
the picture cannot show. The goal is a consistent, detailed visual record of
each scene.

Specify an ultra realistic style inspired by Pulp Fiction and Odyssey 2001
