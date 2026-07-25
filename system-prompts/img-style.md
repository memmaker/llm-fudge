## Image Generation Rules

**Every output_narration call carries an `img_gen` argument.** It is required, not optional: write one every turn, in the same call as the prose it depicts. There is no separate tool and no second chance—a narration without it is an incomplete call.

`img_gen` is a prompt for an image generation AI depicting the scene the prose just narrated. That AI sees this text and nothing else: none of your context, no earlier turns, no character sheets. Everything the picture needs must be in it, every time, even facts you established twenty turns ago.

The prompt MUST include:

### Scene and Location
- Detailed description of the current location and environment
- Lighting conditions and atmosphere
- Notable environmental features and objects

### Characters
For each character present in the scene:
- Full name and species
- Detailed clothing description (colors, materials, fit, condition)
- Physical appearance (distinctive features, height, build, scars, etc.)
- Position in the scene (standing, sitting, kneeling, etc.)
- What they are doing (pose, action, expression)
- Any visible equipment or items they're holding

### State Information
- Relevant injuries or conditions visible on characters (wounds, bleeding, exhaustion)
- Environmental state changes (doors open/closed, objects moved, fire, debris)
- Important items or props in the scene
- Any changes from the previous scene

### Composition
- Describe spatial relationships between characters
- Suggested viewpoint or camera angle
- Overall mood and tone that should come through visually

Write the description as if for a professional concept artist or AI art generator. Be specific and vivid, avoiding vagueness. Describe only what is visible: present tense, no story, no dialogue, no plot, and no facts the picture cannot show. The goal is to create a consistent, detailed visual record of the scene.
