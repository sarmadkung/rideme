import { describe, expect, it, vi } from 'vitest';
import { openEventStream, parseFrame, type StreamEvent } from './stream';

/**
 * A fake XMLHttpRequest that lets a test feed bytes in the shape a real one
 * delivers them: `responseText` grows, and readyState 3 fires repeatedly.
 */
class FakeXhr {
  static last: FakeXhr | null = null;

  readyState = 0;
  status = 0;
  responseText = '';
  aborted = false;
  sentHeaders: Record<string, string> = {};
  url = '';
  onreadystatechange: (() => void) | null = null;
  onerror: (() => void) | null = null;

  constructor() {
    FakeXhr.last = this;
  }

  open(_method: string, url: string) {
    this.url = url;
  }

  setRequestHeader(key: string, value: string) {
    this.sentHeaders[key] = value;
  }

  send() {
    /* the test drives the response */
  }

  abort() {
    this.aborted = true;
    this.readyState = 4;
    this.onreadystatechange?.();
  }

  /** Accepts the subscription. */
  accept(status = 200) {
    this.status = status;
    this.readyState = 2;
    this.onreadystatechange?.();
  }

  /** Appends body bytes, as a progress event would. */
  push(chunk: string) {
    this.responseText += chunk;
    this.readyState = 3;
    this.onreadystatechange?.();
  }

  /** Ends the response. */
  end() {
    this.readyState = 4;
    this.onreadystatechange?.();
  }
}

function frame(event: Partial<StreamEvent> & { type: string }): string {
  const envelope = {
    event_id: 'e1',
    version: 1,
    occurred_at: '2026-09-15T00:00:00Z',
    resource_id: 'job-1',
    payload: {},
    ...event,
  };
  return `event: ${event.type}\ndata: ${JSON.stringify(envelope)}\n\n`;
}

function open(handlers: Parameters<typeof openEventStream>[2]) {
  return openEventStream('https://api.test/api/v1/realtime?channel=job%3Ajob-1', 'tok', handlers, {
    xhr: () => new FakeXhr() as unknown as XMLHttpRequest,
    setTimeout: () => 0,
    clearTimeout: () => {},
  });
}

describe('openEventStream', () => {
  it('delivers each complete frame once', () => {
    const events: StreamEvent[] = [];
    const stream = open({ onEvent: (e) => events.push(e) });

    const xhr = FakeXhr.last!;
    xhr.accept();
    xhr.push(frame({ type: 'job.status_changed' }));
    xhr.push(frame({ type: 'driver.location' }));

    expect(events.map((e) => e.type)).toEqual(['job.status_changed', 'driver.location']);
    stream.close();
  });

  // The common case on a slow connection is half a JSON payload. Parsing it
  // would throw on every chunk.
  it('holds a partial frame until the rest arrives', () => {
    const events: StreamEvent[] = [];
    const stream = open({ onEvent: (e) => events.push(e) });

    const xhr = FakeXhr.last!;
    xhr.accept();
    const whole = frame({ type: 'job.status_changed' });
    xhr.push(whole.slice(0, 20));
    expect(events).toHaveLength(0);

    xhr.push(whole.slice(20));
    expect(events).toHaveLength(1);
    stream.close();
  });

  // Proxies close idle streams, so the server writes comment lines. They are
  // not events and must not be delivered as malformed ones.
  it('ignores heartbeat comments', () => {
    const events: StreamEvent[] = [];
    const stream = open({ onEvent: (e) => events.push(e) });

    const xhr = FakeXhr.last!;
    xhr.accept();
    xhr.push(': subscribed\n\n: keep-alive\n\n');

    expect(events).toHaveLength(0);
    stream.close();
  });

  // A refused subscription is a status code, not a stream. Without this a
  // customer who may not watch a job waits forever on a screen that will never
  // update.
  it('reports a refused subscription', () => {
    const errors: Error[] = [];
    const stream = open({ onEvent: () => {}, onError: (e) => errors.push(e) });

    FakeXhr.last!.accept(403);

    expect(errors).toHaveLength(1);
    expect(errors[0]?.message).toContain('403');
    stream.close();
  });

  // The gateway does not replay (document 047), so a caller recovers what it
  // missed by refetching — which is why onOpen fires on every connection and
  // not only the first.
  it('reconnects when the stream ends, and announces each connection', () => {
    const opens = vi.fn();
    const scheduled: Array<() => void> = [];
    const stream = openEventStream(
      'https://api.test/realtime',
      'tok',
      {
        onEvent: () => {},
        onOpen: opens,
      },
      {
        xhr: () => new FakeXhr() as unknown as XMLHttpRequest,
        setTimeout: (fn) => {
          scheduled.push(fn);
          return scheduled.length;
        },
        clearTimeout: () => {},
      },
    );

    FakeXhr.last!.accept();
    expect(opens).toHaveBeenCalledTimes(1);

    FakeXhr.last!.end();
    expect(scheduled).toHaveLength(1);

    scheduled[0]?.();
    FakeXhr.last!.accept();
    expect(opens).toHaveBeenCalledTimes(2);

    stream.close();
  });

  // close() aborts, and abort fires readyState 4 — which is also what a
  // dropped connection looks like. Reconnecting to a stream the caller let go
  // of leaks a request per screen the customer has already left.
  it('does not reconnect after close', () => {
    const scheduled: Array<() => void> = [];
    const stream = openEventStream(
      'https://api.test/realtime',
      null,
      { onEvent: () => {} },
      {
        xhr: () => new FakeXhr() as unknown as XMLHttpRequest,
        setTimeout: (fn) => {
          scheduled.push(fn);
          return scheduled.length;
        },
        clearTimeout: () => {},
      },
    );

    FakeXhr.last!.accept();
    stream.close();

    expect(FakeXhr.last!.aborted).toBe(true);
    expect(scheduled).toHaveLength(0);
  });

  it('sends the bearer token, and omits it when there is none', () => {
    const withToken = open({ onEvent: () => {} });
    expect(FakeXhr.last!.sentHeaders['Authorization']).toBe('Bearer tok');
    expect(FakeXhr.last!.sentHeaders['Accept']).toBe('text/event-stream');
    withToken.close();

    const anonymous = openEventStream(
      'https://api.test/realtime',
      null,
      { onEvent: () => {} },
      {
        xhr: () => new FakeXhr() as unknown as XMLHttpRequest,
        setTimeout: () => 0,
        clearTimeout: () => {},
      },
    );
    expect(FakeXhr.last!.sentHeaders['Authorization']).toBeUndefined();
    anonymous.close();
  });
});

describe('parseFrame', () => {
  it('returns null for anything that is not an event', () => {
    expect(parseFrame(': keep-alive')).toBeNull();
    expect(parseFrame('')).toBeNull();
    // Malformed JSON is the server's problem and not worth tearing the stream
    // down for.
    expect(parseFrame('event: x\ndata: {not json')).toBeNull();
  });

  it('reads the id line without treating it as data', () => {
    const event = parseFrame(
      'id: abc\nevent: job.status_changed\ndata: {"type":"job.status_changed"}',
    );
    expect(event?.type).toBe('job.status_changed');
  });
});
