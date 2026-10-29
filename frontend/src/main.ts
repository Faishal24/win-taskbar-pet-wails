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

/** Physical pixels per second for a plain walk (45 css px/s at 125%). */
const SPEED = 45 * dpr;
/** Fleeing is ~3.5x walk speed — frantic but trackable. */
const FLEE_SPEED = 160 * dpr;
const FLEE_MS = 1500;
/** Single jump at poke start: height px over HOP_MS, then flat run. */
const FLEE_HOP_PX = 12;
const HOP_MS = 450;

type Pose =
  | 'amble'
  | 'walk'
  | 'run'
  | 'hop'
  | 'stand'
  | 'sleep'
  | 'eat'
  | 'dance'
  | 'wave'
  | 'slip'
  | 'moonwalk'
  | 'backflip'
  | 'shoot';

interface Activity {
  /** Walk-speed multiplier; 0 = stays put. */
  speed: number;
  minMs: number;
  maxMs: number;
  weight: number;
}

// Durations and weights lifted from vscode-petbar (ticks x 160ms), speeds
// kept as multipliers of our walk so the pet keeps its desktop pace.
const ACTIVITIES: Record<Pose, Activity> = {
  amble: { speed: 0.5, minMs: 7200, maxMs: 24000, weight: 20 },
  walk: { speed: 1, minMs: 6400, maxMs: 19200, weight: 20 },
  run: { speed: 2, minMs: 4000, maxMs: 8800, weight: 14 },
  hop: { speed: 1.5, minMs: 3200, maxMs: 7200, weight: 14 },
  stand: { speed: 0, minMs: 4000, maxMs: 11200, weight: 16 },
  sleep: { speed: 0, minMs: 9600, maxMs: 22400, weight: 14 },
  eat: { speed: 0, minMs: 4000, maxMs: 8800, weight: 10 },
  dance: { speed: 0, minMs: 2400, maxMs: 5600, weight: 6 },
  wave: { speed: 0, minMs: 1600, maxMs: 3200, weight: 5 },
  slip: { speed: 2, minMs: 1440, maxMs: 2880, weight: 5 },
  moonwalk: { speed: 0.3, minMs: 5600, maxMs: 12800, weight: 6 },
  backflip: { speed: 1, minMs: 1920, maxMs: 3840, weight: 4 },
  shoot: { speed: 0, minMs: 1600, maxMs: 2880, weight: 6 },
};

const POSE_FRAME_MS: Record<Pose, number> = {
  amble: 170,
  walk: 140,
  run: 90,
  hop: 150,
  stand: 170,
  sleep: 900,
  eat: 500,
  dance: 260,
  wave: 400,
  slip: 220,
  moonwalk: 200,
  backflip: 110,
  shoot: 260,
};

const POSES = Object.keys(ACTIVITIES) as Pose[];
/** Chance to flip travel direction per art pixel walked (vscode: 1%). */
const TURN_CHANCE = 0.01;

let x = 10;
let dir: 1 | -1 = 1;
let hovered = false;
let selected = false;
let fleeUntil = 0;
let fleeStart = -Infinity;
let fleeDir: 1 | -1 = 1;
let activity: Pose = 'walk';
let activityUntil = 0;
let lastActivity: Pose | null = null;
let pose: Pose = 'walk';
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

function currentPose(now: number): Pose {
  if (now < fleeUntil) {
    return 'run';
  }
  return activity;
}

/** Weighted random pick from the vscode activity table. */
function pickActivity(now: number): void {
  let total = 0;
  for (const p of POSES) {
    total += ACTIVITIES[p].weight;
  }
  let picked: Pose = 'walk';
  // Reroll a couple of times so long activities don't repeat back to back.
  for (let attempt = 0; attempt < 3; attempt++) {
    let roll = Math.random() * total;
    picked = 'walk';
    for (const p of POSES) {
      roll -= ACTIVITIES[p].weight;
      if (roll <= 0) {
        picked = p;
        break;
      }
    }
    if (picked !== lastActivity) {
      break;
    }
  }
  startActivity(picked, now);
}

function startActivity(next: Pose, now: number): void {
  const a = ACTIVITIES[next];
  activity = next;
  lastActivity = next;
  activityUntil = now + a.minMs + Math.random() * (a.maxMs - a.minMs);
  pose = next;
  poseStart = now;
  frame = 0;
  api.report(`activity=${next} for ${Math.round((activityUntil - now) / 100) / 10}s`);
}

function facingFor(travelDir: 1 | -1): string {
  // Moonwalk slides backwards: face opposite the travel direction.
  const right = pose === 'moonwalk' ? travelDir === -1 : travelDir === 1;
  return right ? 'right' : 'left';
}

/** Random direction flips keyed on distance walked (1% per art pixel). */
function maybeTurn(distancePx: number): void {
  const cells = distancePx / CELL;
  if (cells <= 0) {
    return;
  }
  if (Math.random() < 1 - Math.pow(1 - TURN_CHANCE, cells)) {
    dir = dir === 1 ? -1 : 1;
  }
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
  const frames = PACK.poses[pose]?.[facingFor(moveDir)] ?? PACK.poses.walk[facingFor(moveDir)];
  if (!frames || frames.length === 0) {
    return;
  }
  const frameMs = POSE_FRAME_MS[pose] ?? 140;
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
    } else {
      if (now >= activityUntil) {
        pickActivity(now);
      }
      const act = ACTIVITIES[activity];
      if (act.speed > 0) {
        const step = SPEED * act.speed * dt;
        maybeTurn(step);
        x += dir * step;
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
