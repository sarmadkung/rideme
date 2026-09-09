import { describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_CAPACITY,
  MutationQueue,
  type QueueStorage,
  type QueuedMutation,
} from './mutationQueue.js';

/** An in-memory store that behaves like the device's, including surviving a "kill". */
function memoryStorage(initial: Record<string, string> = {}): QueueStorage & {
  contents: Record<string, string>;
} {
  const contents: Record<string, string> = { ...initial };
  return {
    contents,
    async getItem(key) {
      return contents[key] ?? null;
    },
    async setItem(key, value) {
      contents[key] = value;
    },
  };
}

function aMutation(overrides: Partial<QueuedMutation> = {}) {
  return {
    id: overrides.id ?? 'm1',
    kind: overrides.kind ?? 'location',
    payload: overrides.payload ?? { latitude: 31.52, longitude: 74.35 },
    idempotencyKey: overrides.idempotencyKey ?? 'key-1',
    ...(overrides.createdAt !== undefined ? { createdAt: overrides.createdAt } : {}),
  };
}

describe('MutationQueue', () => {
  it('sends what was queued while the network was gone', async () => {
    const sent: QueuedMutation[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        sent.push(m);
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    await queue.enqueue(aMutation({ id: 'b' }));
    const result = await queue.flush();

    expect(result.sent).toBe(2);
    expect(sent.map((m) => m.id)).toEqual(['a', 'b']);
    expect(queue.size()).toBe(0);
  });

  it('carries the idempotency key the user acted with, not one made at send time', async () => {
    // A queued booking replayed after reconnect must not create a second job.
    const sent: QueuedMutation[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        sent.push(m);
      },
    });

    await queue.enqueue(aMutation({ id: 'a', idempotencyKey: 'made-when-they-tapped' }));
    await queue.flush();

    expect(sent[0]?.idempotencyKey).toBe('made-when-they-tapped');
  });

  it('survives the app being killed', async () => {
    const storage = memoryStorage();
    const first = new MutationQueue({ storage, send: async () => {} });
    await first.enqueue(aMutation({ id: 'a' }));

    // A second instance is what a cold start looks like.
    const sent: QueuedMutation[] = [];
    const second = new MutationQueue({
      storage,
      send: async (m) => {
        sent.push(m);
      },
    });
    const result = await second.flush();

    expect(result.sent).toBe(1);
    expect(sent[0]?.id).toBe('a');
  });

  it('sends a failed mutation once, not twice', async () => {
    // The failure case is where a queue quietly duplicates work.
    let attempts = 0;
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async () => {
        attempts += 1;
        if (attempts === 1) throw new Error('offline');
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    expect((await queue.flush()).sent).toBe(0);
    expect(queue.size()).toBe(1);

    expect((await queue.flush()).sent).toBe(1);
    expect(attempts).toBe(2);
    expect(queue.size()).toBe(0);
  });

  it('keeps the user’s order when one mutation is stuck', async () => {
    // Delivering a later intent past a stuck earlier one puts them out of
    // sequence, which the server cannot unpick.
    const sent: string[] = [];
    let failFirst = true;
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        if (m.id === 'a' && failFirst) throw new Error('offline');
        sent.push(m.id);
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    await queue.enqueue(aMutation({ id: 'b' }));
    await queue.flush();
    expect(sent).toEqual([]);

    failFirst = false;
    await queue.flush();
    expect(sent).toEqual(['a', 'b']);
  });

  it('drops a mutation that is too old to mean anything', async () => {
    // An hour-old location fix is not stale data, it is a wrong answer.
    let now = 1_000_000;
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        sent.push(m.id);
      },
      maxAgeMsByKind: { location: 5 * 60_000 },
      now: () => now,
    });

    await queue.enqueue(aMutation({ id: 'old', kind: 'location' }));
    now += 6 * 60_000;
    await queue.enqueue(aMutation({ id: 'fresh', kind: 'location' }));

    const result = await queue.flush();
    expect(result.expired).toBe(1);
    expect(sent).toEqual(['fresh']);
  });

  it('never expires a kind nobody has ruled on', async () => {
    // The platform does not invent a rule about when a user's intent stops
    // counting. A kind with no configured age is kept.
    let now = 1_000_000;
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        sent.push(m.id);
      },
      maxAgeMsByKind: { location: 60_000 },
      now: () => now,
    });

    await queue.enqueue(aMutation({ id: 'booking', kind: 'booking' }));
    now += 24 * 3600_000;

    expect((await queue.flush()).expired).toBe(0);
    expect(sent).toEqual(['booking']);
  });

  it('is bounded, dropping the oldest first', async () => {
    // An unbounded queue replays stale intent and grows the stored blob on a
    // phone that cannot afford it.
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        sent.push(m.id);
      },
      capacity: 3,
    });

    for (const id of ['a', 'b', 'c', 'd', 'e']) {
      await queue.enqueue(aMutation({ id }));
    }
    expect(queue.size()).toBe(3);

    await queue.flush();
    // The newest intent is the likeliest to still be true.
    expect(sent).toEqual(['c', 'd', 'e']);
  });

  it('has a bound even when nobody sets one', async () => {
    expect(DEFAULT_CAPACITY).toBeGreaterThan(0);
    expect(Number.isFinite(DEFAULT_CAPACITY)).toBe(true);
  });

  it('does not send the same mutation twice when two flushes race', async () => {
    // A reconnect event and a foreground event arriving together is ordinary.
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async (m) => {
        await new Promise((resolve) => setTimeout(resolve, 5));
        sent.push(m.id);
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    await Promise.all([queue.flush(), queue.flush(), queue.flush()]);

    expect(sent).toEqual(['a']);
  });

  it('treats an unreadable store as an empty one', async () => {
    // Refusing to start because a cache will not parse takes the app down for
    // data it can live without.
    const storage = memoryStorage({ 'rideme.mutation-queue.v1': 'not json{{' });
    const queue = new MutationQueue({ storage, send: async () => {} });

    await queue.load();
    expect(queue.size()).toBe(0);
  });

  it('discards entries that are not mutations', async () => {
    const storage = memoryStorage({
      'rideme.mutation-queue.v1': JSON.stringify([
        { nonsense: true },
        {
          id: 'a',
          kind: 'location',
          payload: {},
          idempotencyKey: 'k',
          createdAt: 1,
          attempts: 0,
        },
      ]),
    });
    const queue = new MutationQueue({ storage, send: async () => {} });

    await queue.load();
    expect(queue.size()).toBe(1);
  });

  it('keeps working in memory when the store cannot be written', async () => {
    // Losing the queue to an app kill is bad; refusing the user's action is
    // worse.
    const storage: QueueStorage = {
      getItem: async () => null,
      setItem: async () => {
        throw new Error('disk full');
      },
    };
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage,
      send: async (m) => {
        sent.push(m.id);
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    expect((await queue.flush()).sent).toBe(1);
    expect(sent).toEqual(['a']);
  });

  it('forgets everything on sign-out', async () => {
    // A queue holding one account's intent must never flush under another's
    // session.
    const storage = memoryStorage();
    const sent: string[] = [];
    const queue = new MutationQueue({
      storage,
      send: async (m) => {
        sent.push(m.id);
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    await queue.clear();
    await queue.flush();

    expect(sent).toEqual([]);
    expect(queue.size()).toBe(0);
    // And it does not come back on a cold start.
    const reloaded = new MutationQueue({ storage, send: async () => {} });
    await reloaded.load();
    expect(reloaded.size()).toBe(0);
  });

  it('counts attempts so a caller can back off or give up', async () => {
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: async () => {
        throw new Error('offline');
      },
    });

    await queue.enqueue(aMutation({ id: 'a' }));
    await queue.flush();
    await queue.flush();

    expect(queue.peek()[0]?.attempts).toBe(2);
  });

  it('reports what happened, so the UI can show real state', async () => {
    // A UI identical online and offline gets a driver stranded believing a job
    // was accepted.
    let now = 0;
    const queue = new MutationQueue({
      storage: memoryStorage(),
      send: vi.fn(async () => {}),
      maxAgeMsByKind: { location: 10 },
      now: () => now,
    });

    await queue.enqueue(aMutation({ id: 'stale', kind: 'location' }));
    now = 100;
    await queue.enqueue(aMutation({ id: 'fresh', kind: 'location' }));

    expect(await queue.flush()).toEqual({ sent: 1, expired: 1, remaining: 0 });
  });
});
