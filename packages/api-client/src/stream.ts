/**
 * The client half of the realtime gateway (documents 018, 047).
 *
 * The server grew a Server-Sent Events transport; nothing consumed it. Until
 * this existed the customer app followed a live job by re-reading it every
 * five seconds, and the comment in `useBooking` said why: "the realtime
 * gateway exists but the client transport for it does not".
 *
 * Built on XMLHttpRequest rather than EventSource. React Native has no
 * EventSource, and the one thing every SSE library for React Native has in
 * common is that it reads an XHR's incremental `responseText` — which is what
 * this does, in about eighty lines, with no dependency to keep current.
 */

/** One event, exactly as `realtime.Envelope` serialises it. */
export interface StreamEvent<T = unknown> {
  event_id: string;
  type: string;
  version: number;
  occurred_at: string;
  resource_id: string;
  payload: T;
}

export interface StreamHandlers {
  onEvent(event: StreamEvent): void;
  /** Called on every (re)connection, so a caller can refetch what it missed. */
  onOpen?(): void;
  onError?(error: Error): void;
}

export interface StreamOptions {
  /** Injected so a test can drive the transport without a network. */
  xhr?: () => XMLHttpRequest;
  /** Injected so a test does not wait out a backoff. */
  setTimeout?: (fn: () => void, ms: number) => unknown;
  clearTimeout?: (handle: unknown) => void;
}

export interface EventStream {
  close(): void;
}

/** Reconnection backoff, doubling to a ceiling. */
const FIRST_RETRY_MS = 1_000;
export const MAX_RETRY_MS = 30_000;

/**
 * Opens a stream and keeps it open.
 *
 * Reconnection is the whole reason this is not four lines. A phone loses its
 * connection in a lift, a proxy closes an idle socket, the app comes back from
 * the background — all of which end the request, and a screen that silently
 * stops updating after the first of them is worse than one that never
 * streamed. `onOpen` fires on each reconnection so the caller can refetch
 * state: this gateway does not replay (document 047), so what was missed is
 * recovered by asking, not by resuming.
 */
export function openEventStream(
  url: string,
  token: string | null,
  handlers: StreamHandlers,
  options: StreamOptions = {},
): EventStream {
  const makeXhr = options.xhr ?? (() => new XMLHttpRequest());
  const schedule = options.setTimeout ?? ((fn, ms) => globalThis.setTimeout(fn, ms));
  const unschedule = options.clearTimeout ?? ((h) => globalThis.clearTimeout(h as never));

  let closed = false;
  let request: XMLHttpRequest | null = null;
  let retryMs = FIRST_RETRY_MS;
  let retryHandle: unknown = null;

  function connect(): void {
    if (closed) return;

    const xhr = makeXhr();
    request = xhr;
    // How much of responseText has already been turned into events. XHR gives
    // the whole body so far on every progress event, not the delta.
    let consumed = 0;
    let opened = false;

    xhr.onreadystatechange = () => {
      if (closed) return;
      // HEADERS_RECEIVED. A refused subscription is a status code, not a
      // stream: reporting it is what tells a caller it may not watch this job,
      // rather than leaving a screen waiting for events that will never come.
      if (xhr.readyState === 2 && !opened) {
        if (xhr.status >= 200 && xhr.status < 300) {
          opened = true;
          retryMs = FIRST_RETRY_MS;
          handlers.onOpen?.();
        } else if (xhr.status !== 0) {
          handlers.onError?.(new Error(`realtime stream refused: ${xhr.status}`));
        }
      }
      if (xhr.readyState === 3 || xhr.readyState === 4) {
        consumed = drain(xhr.responseText ?? '', consumed, handlers);
      }
      if (xhr.readyState === 4) {
        // The server closed, or the connection dropped. Either way the stream
        // is over and the only difference is how long to wait.
        reconnect();
      }
    };
    xhr.onerror = () => {
      if (!closed) reconnect();
    };

    xhr.open('GET', url, true);
    xhr.setRequestHeader('Accept', 'text/event-stream');
    if (token !== null) xhr.setRequestHeader('Authorization', `Bearer ${token}`);
    xhr.send();
  }

  function reconnect(): void {
    if (closed) return;
    request = null;
    const wait = retryMs;
    retryMs = Math.min(retryMs * 2, MAX_RETRY_MS);
    retryHandle = schedule(connect, wait);
  }

  connect();

  return {
    close(): void {
      closed = true;
      if (retryHandle !== null) unschedule(retryHandle);
      // abort() fires onreadystatechange with readyState 4; `closed` is set
      // first so that does not schedule a reconnection to a stream the caller
      // has just let go of.
      request?.abort();
      request = null;
    },
  };
}

/**
 * Turns whatever is complete in the buffer into events, and reports how much
 * was consumed.
 *
 * Frames are separated by a blank line, so a partial frame at the end is left
 * for the next progress event — the common case on a slow connection is half a
 * JSON payload, and parsing it would throw on every chunk.
 */
function drain(buffer: string, from: number, handlers: StreamHandlers): number {
  let consumed = from;
  for (;;) {
    const end = buffer.indexOf('\n\n', consumed);
    if (end === -1) return consumed;
    const frame = buffer.slice(consumed, end);
    consumed = end + 2;
    const event = parseFrame(frame);
    if (event !== null) handlers.onEvent(event);
  }
}

/** Parses one SSE frame. Comment lines (`: keep-alive`) yield nothing. */
export function parseFrame(frame: string): StreamEvent | null {
  let data = '';
  for (const line of frame.split('\n')) {
    if (line === '' || line.startsWith(':')) continue;
    if (line.startsWith('data:')) data += line.slice(5).trimStart();
  }
  if (data === '') return null;
  try {
    return JSON.parse(data) as StreamEvent;
  } catch {
    // A malformed frame is the server's problem and not worth tearing the
    // stream down for: the next event is usually fine, and this path is not
    // the system of record.
    return null;
  }
}
