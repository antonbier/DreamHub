// The virtual remote's keys: standard Linux input-event codes, which is
// what the box's /web/remotecontrol?command= expects (ARCHITECTURE.md,
// "Real screenshot and remote-control endpoints"). The arrows, OK, EXIT,
// MENU, the four color keys and POWER are confirmed live on the reference
// box; every other key here is checked against the box's own
// /usr/share/enigma2/keymap.xml (ARCHITECTURE.md, "Remote keys the box
// handles") — which has no KEY_EPG at all: on a Dreambox, INFO opens the
// programme info, so there's no EPG key here either. A key the box's
// current screen doesn't handle is harmless: the box briefly shows a
// "blocked" icon and ignores it, like an unmapped button on a physical
// remote.
export const KEY = {
  POWER: 116,
  MUTE: 113,
  VOLUME_UP: 115,
  VOLUME_DOWN: 114,
  CHANNEL_UP: 402,
  CHANNEL_DOWN: 403,
  UP: 103,
  DOWN: 108,
  LEFT: 105,
  RIGHT: 106,
  OK: 352,
  EXIT: 174,
  MENU: 139,
  INFO: 358,
  TEXT: 388,
  AUDIO: 392,
  TV: 377,
  RADIO: 385,
  RED: 398,
  GREEN: 399,
  YELLOW: 400,
  BLUE: 401,
  REWIND: 168,
  PLAY: 207,
  PAUSE: 119,
  STOP: 128,
  FAST_FORWARD: 208,
  DIGITS: [11, 2, 3, 4, 5, 6, 7, 8, 9, 10], // KEY_0, KEY_1 … KEY_9
} as const

// Physical-keyboard shortcuts while the remote has focus. Escape is left
// out on purpose: it closes the side panel (DESIGN.md, "Esc closes
// dialogs, menus and the drawer"), so Backspace is the remote's EXIT.
export function keyForKeyboardEvent(e: KeyboardEvent): number | undefined {
  if (e.metaKey || e.ctrlKey || e.altKey) return undefined
  if (/^[0-9]$/.test(e.key)) return KEY.DIGITS[Number(e.key)]
  switch (e.key) {
    case 'ArrowUp':
      return KEY.UP
    case 'ArrowDown':
      return KEY.DOWN
    case 'ArrowLeft':
      return KEY.LEFT
    case 'ArrowRight':
      return KEY.RIGHT
    case 'Enter':
      return KEY.OK
    case 'Backspace':
      return KEY.EXIT
    case 'PageUp':
      return KEY.CHANNEL_UP
    case 'PageDown':
      return KEY.CHANNEL_DOWN
    case '+':
      return KEY.VOLUME_UP
    case '-':
      return KEY.VOLUME_DOWN
    case 'm':
      return KEY.MENU
    case 'i':
      return KEY.INFO
  }
  return undefined
}
