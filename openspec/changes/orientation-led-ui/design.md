## Decisions

1. **Orientation is a pad setting, not per profile.** It describes the physical
   placement; stored in app preferences. `Keys[0..2]` always mean the keys as seen:
   left->right when horizontal, top->bottom when vertical. The device tool remaps
   for us; we only emit `orientation:` and the grid shape (horizontal: 1 row of 3,
   vertical: 3 rows of 1, verified with `ch57x-keyboard-tool validate`).
   The `rows`/`columns` header always describes the physical board (`1`/`3`);
   the tool rejects `rows: 3, columns: 1` for the rotated orientations.
   Knob position as seen: normal=right, upsidedown=left, clockwise=bottom,
   counterclockwise=top.
2. **LED capabilities per model** via `pad.CapsFor(productID)`:
   - `8890`: global (not per layer), modes by index 0..5 with labels; index 0 is
     off. Labels beyond 0 are best-effort and editable in the UI because the
     firmware is undocumented (`led <n>`).
   - `8840/8842`: per layer (0..2); effects `off|backlight|shock|shock2|press`;
     colours `red orange yellow green cyan blue purple` (+`white` for backlight)
     (`led <layer> "<effect>[ <color>]"`: effect and colour are ONE argv element,
     e.g. `led 0 "backlight red"`; tool v1.7.0 has a wrong 884x packet layout,
     upstream issue #180, fixed on master only).
3. `pad.Writer.SetLED(LEDSetting)` replaces `SetLED(int)`; `LEDController` works on
   `LEDSetting` values (comparable struct) with the same change-only + rate limit rules.
4. **Responsive UI**: a breakpoint layout switches between side navigation (wide)
   and top tabs/compact cards (narrow, < 720 px); the pad drawing scales to the
   available space; long lists scroll inside their card, not the whole page.
   Device writes run on a single worker goroutine fed by a channel, so neither
   the UI goroutine nor the HTTP receiver ever waits for USB.
