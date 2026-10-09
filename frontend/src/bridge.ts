/**
 * Bridge to the Go backend. Uses the Wails runtime globals directly
 * (window.go / window.runtime) instead of generated wailsjs bindings, so
 * this file works without running any codegen step.
 *
 * Ownership split (same as the Electron version):
 * - backend owns transparency, hover detection and click-anywhere unselect
 *   (it polls the global mouse and knows the monkey rect we push);
 * - frontend renders, walks, pokes on real pointerdown, reports state.
 */
declare global {
  interface Window {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    go: any;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    runtime: any;
  }
}

export interface PetApi {
  report(message: string): void;
  pushRect(x: number, y: number, w: number, h: number, selected: boolean): void;
  onUnselect(cb: () => void): void;
  onPaused(cb: (paused: boolean) => void): void;
  onHover(cb: (hovered: boolean) => void): void;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function backend(): any {
  try {
    return window.go?.main?.App;
  } catch {
    return undefined;
  }
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function runtime(): any {
  try {
    return window.runtime;
  } catch {
    return undefined;
  }
}

function onEvent(name: string, cb: (...args: never[]) => void): void {
  try {
    runtime()?.EventsOn(name, cb);
  } catch {
    // not running inside Wails (plain browser preview) — ignore
  }
}

export const api: PetApi = {
  report(message: string): void {
    console.log('[pet]', message);
    try {
      backend()?.Log(message);
    } catch {
      // ignore
    }
  },
  pushRect(x: number, y: number, w: number, h: number, selected: boolean): void {
    try {
      backend()?.UpdateRect(x, y, w, h, selected);
    } catch {
      // ignore
    }
  },
  onUnselect(cb: () => void): void {
    onEvent('pet-unselect', cb);
  },
  onPaused(cb: (paused: boolean) => void): void {
    onEvent('pet-paused', (v: boolean) => cb(!!v));
  },
  onHover(cb: (hovered: boolean) => void): void {
    onEvent('pet-hover', (v: boolean) => cb(!!v));
  },
};
