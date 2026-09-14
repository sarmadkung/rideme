/**
 * A durable queue for mutations made while the network is gone.
 *
 * Connectivity on this platform's market is not a rare failure; it is a normal
 * condition. A driver loses signal in a basement car park and regains it two
 * streets later, and what happened in between must not be lost — nor replayed
 * as though it were still true.
 *
 * Three rules from `mobile-offline-sync` shape everything here:
 *
 *   - The server is authoritative. This queue delivers intent; it never
 *     decides an outcome.
 *   - A queued mutation carries the idempotency key generated when the user
 *     acted, not when it is finally sent. Replaying a booking after reconnect
 *     must not create a second job.
 *   - The queue is bounded, in both size and age. An unbounded queue replays
 *     stale intent, and an hour-old intent is usually wrong to send.
 */

/** Where the queue survives an app kill. Injected so this package needs no native module. */
export interface QueueStorage {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
}

/**
 * What a queued mutation is.
 *
 * `kind` is a string rather than a union so the queue does not have to know
 * every caller's vocabulary; the caller's sender switches on it.
 */
export interface QueuedMutation {
  id: string;
  kind: string;
  payload: unknown;
  /**
   * Generated when the user acted. Carried so a replay is the same request,
   * not a second one (document 377).
   */
  idempotencyKey: string;
  /** When the user acted, in epoch milliseconds. Used for expiry. */
  createdAt: number;
  attempts: number;
}

/** Delivers one mutation. Resolving means the server accepted it. */
export type Sender = (mutation: QueuedMutation) => Promise<void>;

export interface QueueOptions {
  storage: QueueStorage;
  send: Sender;
  /**
   * Oldest a mutation may be and still be worth sending.
   *
   * Per kind, because the answer genuinely differs: a location fix is
   * worthless within minutes, while a booking a customer made is not.
   * There is no default, and a kind with no entry is never expired — the
   * platform must not invent a rule about discarding a user's intent.
   */
  maxAgeMsByKind?: Readonly<Record<string, number>>;
  /** How many mutations may wait. Oldest are dropped first. */
  capacity?: number;
  now?: () => number;
  storageKey?: string;
}

/**
 * How many mutations may wait before the oldest are dropped.
 *
 * A bound is required; the number is an engineering default. Two hundred is
 * far more than a person generates in a tunnel and far less than enough to
 * make the stored blob a problem on a low-end phone.
 */
export const DEFAULT_CAPACITY = 200;

const DEFAULT_STORAGE_KEY = 'rideme.mutation-queue.v1';

export interface FlushResult {
  sent: number;
  /** Dropped for age before any attempt was made. */
  expired: number;
  /** Still queued because the send failed. */
  remaining: number;
}

export class MutationQueue {
  private readonly storage: QueueStorage;
  private readonly send: Sender;
  private readonly maxAgeMsByKind: Readonly<Record<string, number>>;
  private readonly capacity: number;
  private readonly now: () => number;
  private readonly storageKey: string;

  private pending: QueuedMutation[] = [];
  private loaded = false;
  /** Serialises flushes so one cannot send what another is already sending. */
  private inFlight: Promise<FlushResult> | null = null;

  constructor(options: QueueOptions) {
    this.storage = options.storage;
    this.send = options.send;
    this.maxAgeMsByKind = options.maxAgeMsByKind ?? {};
    this.capacity = options.capacity ?? DEFAULT_CAPACITY;
    this.now = options.now ?? Date.now;
    this.storageKey = options.storageKey ?? DEFAULT_STORAGE_KEY;
  }

  /**
   * Reads what survived the last app kill.
   *
   * A corrupt or unreadable store is treated as an empty one. Refusing to
   * start because a cache will not parse would take the app down for the sake
   * of data it can live without.
   */
  async load(): Promise<void> {
    if (this.loaded) return;
    this.loaded = true;
    try {
      const raw = await this.storage.getItem(this.storageKey);
      if (raw === null) return;
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed)) return;
      this.pending = parsed.filter(isQueuedMutation);
    } catch {
      this.pending = [];
    }
  }

  /** What is waiting. */
  size(): number {
    return this.pending.length;
  }

  /** A copy of what is waiting, for a UI that must show pending work. */
  peek(): readonly QueuedMutation[] {
    return [...this.pending];
  }

  /**
   * Queues one mutation.
   *
   * The idempotency key is the caller's, generated when the user acted. This
   * signature takes it rather than making one, so it cannot accidentally be
   * created at send time — which would turn one booking into two.
   */
  async enqueue(
    mutation: Omit<QueuedMutation, 'createdAt' | 'attempts'> & { createdAt?: number },
  ): Promise<void> {
    await this.load();
    this.pending.push({
      ...mutation,
      createdAt: mutation.createdAt ?? this.now(),
      attempts: 0,
    });
    // Oldest first: the newest intent is the likeliest to still be true.
    if (this.pending.length > this.capacity) {
      this.pending = this.pending.slice(this.pending.length - this.capacity);
    }
    await this.persist();
  }

  /**
   * Sends everything that is still worth sending.
   *
   * Stops at the first failure and keeps the rest. Order is the order the user
   * acted in, and sending a later mutation past a stuck earlier one would
   * deliver their intent out of sequence.
   *
   * Concurrent calls share one flush. Two flushes racing — a reconnect event
   * and a foreground event arriving together — would send the same mutation
   * twice, and idempotency keys make that survivable rather than free.
   */
  async flush(): Promise<FlushResult> {
    if (this.inFlight !== null) return this.inFlight;
    this.inFlight = this.run();
    try {
      return await this.inFlight;
    } finally {
      this.inFlight = null;
    }
  }

  private async run(): Promise<FlushResult> {
    await this.load();

    const fresh: QueuedMutation[] = [];
    let expired = 0;
    for (const mutation of this.pending) {
      if (this.hasExpired(mutation)) {
        expired += 1;
        continue;
      }
      fresh.push(mutation);
    }
    this.pending = fresh;

    let sent = 0;
    while (this.pending.length > 0) {
      const next = this.pending[0];
      if (next === undefined) break;
      try {
        await this.send(next);
      } catch {
        // Keep it, count the attempt, and stop. The next reconnect tries
        // again; retry policy belongs to whoever calls flush.
        next.attempts += 1;
        break;
      }
      this.pending.shift();
      sent += 1;
    }

    await this.persist();
    return { sent, expired, remaining: this.pending.length };
  }

  /**
   * Forgets everything.
   *
   * Used on sign-out: a queue holding one account's intent must never be
   * flushed under another's session.
   */
  async clear(): Promise<void> {
    this.loaded = true;
    this.pending = [];
    await this.persist();
  }

  private hasExpired(mutation: QueuedMutation): boolean {
    const maxAge = this.maxAgeMsByKind[mutation.kind];
    // No configured age means no expiry. The platform does not invent a rule
    // about when a user's intent stops counting.
    if (maxAge === undefined) return false;
    return this.now() - mutation.createdAt > maxAge;
  }

  private async persist(): Promise<void> {
    try {
      await this.storage.setItem(this.storageKey, JSON.stringify(this.pending));
    } catch {
      // A queue that cannot be written is still a queue in memory. Losing it
      // to an app kill is bad; refusing the user's action is worse.
    }
  }
}

function isQueuedMutation(value: unknown): value is QueuedMutation {
  if (typeof value !== 'object' || value === null) return false;
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    typeof candidate['kind'] === 'string' &&
    typeof candidate['idempotencyKey'] === 'string' &&
    typeof candidate['createdAt'] === 'number' &&
    typeof candidate['attempts'] === 'number'
  );
}
