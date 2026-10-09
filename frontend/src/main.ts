/**
 * Taskbar pet renderer (Wails port): the classic pack from vscode-petbar —
 * walk cycle while strolling, stand/blink cycle during idle pauses,
 * bounce-run away on poke. Full-width roam above the taskbar.
 *
 * Rendering is done in integer PHYSICAL pixels (identity transform): at
 * 125% scale any CSS-unit drawing lands on fractional physical pixels and
 * every cell edge gets antialiased (smooth grid + shimmer). Physical pixels
 * keep the art crisp and still.
 *
 * Input split: the Go backend owns transparency + hover detection (it polls
 * the global mouse and knows the rect we push), so real pointerdown events
 * drive poke locally while hover/unselect/pause arrive as backend events.
 */
import rawSprites from './sprites.gen.json';
import { api } from './bridge';

interface GridFrame {
  rows: string[];
  dy: number;
}

interface SpritePack {
  palette: Record<string, string>;
  poses: Record<string, Record<string, GridFrame[]>>;
}

interface SpriteData {
  packs: Record<string, SpritePack>;
}

const SPRITES = rawSprites as unknown as SpriteData;
const PACK = SPRITES.packs.classic;
const ART_W = 24;
const ART_H = 16;
/** Integer physical pixels: the only crisp cell size on any scale factor. */
const CELL = 2;

const canvas = document.getElementById('pet') as HTMLCanvasElement;
const ctx = canvas.getContext('2d') as CanvasRenderingContext2D;

let dpr = 1;

function resize(): void {
  dpr = window.devicePixelRatio || 1;
  canvas.width = Math.max(1, Math.round(window.innerWidth * dpr));
  canvas.height = Math.max(1, Math.round(window.innerHeight * dpr));
  // Identity transform: all drawing below uses integer physical pixels.
  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.imageSmoothingEnabled = false;
}
window.addEventListener('resize', resize);
resize();

/** Physical pixels per second (45 css px/s at 125% scale). */
const SPEED = 45 * dpr;
/** Fleeing is ~3.5x walk speed — frantic but trackable. */
const FLEE_SPEED = 160 * dpr;
const FLEE_MS = 1500;
/** Single jump at poke start: height px over HOP_MS, then flat run. */
const FLEE_HOP_PX = 12;
const HOP_MS = 450;
const WALK_FRAME_MS = 140;
const RUN_FRAME_MS = 90;
const STAND_FRAME_MS = 170;
const IDLE_MS = 1400;
const NEXT_IDLE_MIN_MS = 4000;
const NEXT_IDLE_SPAN_MS = 4000;

let x = 10;
let dir: 1 | -1 = 1;
let hovered = false;
let selected = false;
let idleUntil = 0;
let fleeUntil = 0;
let fleeStart = -Infinity;
let fleeDir: 1 | -1 = 1;
let nextIdleAt = performance.now() + 3000;
let pose = 'walk';
let poseStart = performance.now();
let frame = 0;
let paused = false;

api.onPaused((value) => {
  paused = value;
});

api.onHover((value) => {
  if (value === hovered) {
    return;
  }
  hovered = value;
  canvas.style.cursor = value ? 'pointer' : 'default';
});

function groundY(): number {
  return canvas.height - 3;
}

function currentPose(now: number): string {
  if (now < fleeUntil) {
    return 'run';
  }
  if (now < idleUntil) {
    return 'stand';
  }
  return 'walk';
}

function facing(moveDir: 1 | -1): string {
  return moveDir === 1 ? 'right' : 'left';
}

function monkeyRect(now: number): { px: number; py: number; w: number; h: number; hop: number } {
  const w = ART_W * CELL;
  const h = ART_H * CELL;
  const t = (now - fleeStart) / HOP_MS;
  const hop = t >= 0 && t < 1 ? Math.sin(t * Math.PI) * FLEE_HOP_PX : 0;
  return { px: x, py: groundY() - h, w, h, hop };
}

function hitTest(mxCss: number, myCss: number, now: number): boolean {
  const mx = mxCss * dpr;
  const my = myCss * dpr;
  const m = monkeyRect(now);
  const top = m.py - m.hop;
  const pad = 5;
  return (
    mx >= m.px - pad &&
    mx <= m.px + m.w + pad &&
    my >= top - pad &&
    my <= m.py + m.h + pad
  );
}

function drawFrame(f: GridFrame, xPx: number, feet: number, lift: number): void {
  // All coordinates are integers by construction (see draw()), so every
  // cell lands exactly on physical pixels — no antialiased edges.
  for (let r = 0; r < f.rows.length; r++) {
    const row = f.rows[r];
    const y = Math.round(feet - (f.rows.length - r + f.dy + lift) * CELL);
    for (let c = 0; c < row.length; c++) {
      const ch = row[c];
      if (ch === '.') {
        continue;
      }
      ctx.fillStyle = PACK.palette[ch] ?? '#ffffff';
      ctx.fillRect(Math.round(xPx + c * CELL), y, CELL, CELL);
    }
  }
}

function draw(now: number): void {
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  const m = monkeyRect(now);
  const feet = Math.round(groundY() - m.hop);
  const px = Math.round(m.px);

  // Shadow stays on the ground while the monkey bounces away.
  ctx.fillStyle = 'rgba(0, 0, 0, 0.35)';
  const sw = Math.round(m.w * 0.7);
  ctx.fillRect(Math.round(m.px + (m.w - sw) / 2), groundY(), sw, 2);

  if (selected) {
    // Selection ring marker at the feet (original vscode-petbar style):
    // a ground line under the monkey with ticks at both ends. Toggled by
    // clicking the monkey.
    const gy = groundY();
    const rw = Math.round(m.w);
    ctx.fillStyle = 'rgba(255, 214, 51, 0.9)';
    ctx.fillRect(px, gy - 1, rw, 2);
    ctx.fillRect(px, gy - 4, 3, 3);
    ctx.fillRect(px + rw - 3, gy - 4, 3, 3);
  }

  const moveDir = now < fleeUntil ? fleeDir : dir;
  const frames = PACK.poses[pose]?.[facing(moveDir)] ?? PACK.poses.walk[facing(moveDir)];
  if (!frames || frames.length === 0) {
    return;
  }
  const frameMs = pose === 'walk' ? WALK_FRAME_MS : pose === 'run' ? RUN_FRAME_MS : STAND_FRAME_MS;
  // The first rAF timestamp can predate module-eval performance.now() by a
  // hair; plain % keeps the sign and would index frames[-1] -> crash.
  const count = frames.length;
  frame = ((Math.floor((now - poseStart) / frameMs) % count) + count) % count;
  drawFrame(frames[frame], px, feet, 0);
}

canvas.addEventListener('pointerdown', (event) => {
  const now = performance.now();
  if (hitTest(event.clientX, event.clientY, now)) {
    // Poke: bounce-run away from the click point, then resume strolling.
    const m = monkeyRect(now);
    fleeDir = event.clientX * dpr < m.px + m.w / 2 ? 1 : -1;
    dir = fleeDir;
    fleeStart = now;
    fleeUntil = now + FLEE_MS;
    nextIdleAt = now + FLEE_MS + 2000 + Math.random() * 4000;
    selected = !selected;
    api.report(`poke! flee ${dir > 0 ? 'right' : 'left'}${selected ? ' (selected)' : ''}`);
  }
});

let last = performance.now();
let frames = 0;
let fpsMark = performance.now();
const FRAME_MS = 1000 / 60;
let lastDraw = -Infinity;

// Click-anywhere unselect arrives from the backend (it watches global mouse
// presses; we never see clicks outside the monkey — clickthrough).
api.onUnselect(() => {
  if (selected) {
    selected = false;
    api.report('unselected (outside click)');
  }
});

let lastRectPush = 0;

function loop(now: number): void {
  requestAnimationFrame(loop);
  // Cap at display rate: uncapped rAF overheats software rendering.
  // Skipped ticks do NOT advance `last`, so speed is preserved.
  if (now - lastDraw < FRAME_MS) {
    return;
  }
  lastDraw = now;
  const dt = Math.max(0, Math.min(0.1, (now - last) / 1000));
  last = now;

  if (!paused) {
    if (now < fleeUntil) {
      x += fleeDir * FLEE_SPEED * dt;
    } else if (now >= idleUntil) {
      if (now >= nextIdleAt) {
        idleUntil = now + IDLE_MS;
        nextIdleAt = now + IDLE_MS + NEXT_IDLE_MIN_MS + Math.random() * NEXT_IDLE_SPAN_MS;
      } else {
        x += dir * SPEED * dt;
      }
    }
    const maxX = Math.max(0, canvas.width - ART_W * CELL);
    if (x >= maxX) {
      x = maxX;
      dir = -1;
      if (now < fleeUntil) {
        fleeDir = -1;
      }
    } else if (x <= 0) {
      x = 0;
      dir = 1;
      if (now < fleeUntil) {
        fleeDir = 1;
      }
    }
  }

  const wanted = paused ? 'stand' : currentPose(now);
  if (wanted !== pose) {
    pose = wanted;
    poseStart = now;
    frame = 0;
  }
  if (paused) {
    frame = 0;
  }

  draw(now);

  frames += 1;
  if (now - fpsMark >= 1000) {
    api.report(`fps=${frames} view=${canvas.width}x${canvas.height} dpr=${dpr} pose=${pose}`);
    frames = 0;
    fpsMark = now;
  }
  // Push monkey rect (window-client physical px) for the backend
  // click-anywhere watcher. 10Hz is plenty (monkey moves ~6px per push).
  if (now - lastRectPush >= 100) {
    lastRectPush = now;
    const m = monkeyRect(now);
    api.pushRect(Math.round(m.px), Math.round(m.py), Math.round(m.w), Math.round(m.h), selected);
  }
}

api.report(`renderer up view=${canvas.width}x${canvas.height} dpr=${dpr} cell=${CELL}`);
requestAnimationFrame(loop);

export {};
