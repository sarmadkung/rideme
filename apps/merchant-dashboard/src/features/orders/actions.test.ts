import { describe, expect, it } from 'vitest';
import { ApiError } from '@platform/api-client';
import type { MerchantOrder } from '@platform/types';
import { availableActions, describeFailure, minutesLeft, statusLabel } from './actions';

function order(fields: Partial<MerchantOrder>): MerchantOrder {
  return {
    id: 'o1',
    status: 'PLACED',
    items_total: { amount_minor: 1000, currency: 'PKR' },
    created_at: '2026-09-11T09:00:00Z',
    ...fields,
  } as MerchantOrder;
}

describe('availableActions', () => {
  it('offers accept and reject only while the order is waiting on an answer', () => {
    expect(availableActions('PLACED')).toEqual(['accept', 'reject']);
  });

  it('never offers accept on an order that is waiting on payment', () => {
    // The server refuses it with "this order is waiting on payment" — that is
    // a question for the payment flow, and a shop tapping Accept on it gets an
    // error it can do nothing about. Reject stays: a shop that cannot fill the
    // order should not have to wait for a payment to succeed first.
    expect(availableActions('PAYMENT_PENDING')).toEqual(['reject']);
  });

  it('moves from preparing to ready, and offers reporting an issue only while picking', () => {
    expect(availableActions('CONFIRMED')).toEqual(['preparing', 'reject']);
    expect(availableActions('PREPARING')).toEqual(['ready', 'issue']);
    // Reporting an empty shelf after the order has left the shop is refused
    // server-side (ErrNotPicking), so it is not offered.
    expect(availableActions('READY_FOR_PICKUP')).toEqual([]);
  });

  it('offers nothing once the order has left the shop or ended', () => {
    for (const status of ['PICKED_UP', 'DELIVERING', 'DELIVERED', 'CANCELLED', 'FAILED']) {
      expect(availableActions(status)).toEqual([]);
    }
  });
});

describe('minutesLeft', () => {
  const now = Date.parse('2026-09-11T09:00:00Z');

  it('counts whole minutes to the accept deadline', () => {
    expect(minutesLeft(order({ accept_deadline: '2026-09-11T09:04:30Z' }), now)).toBe(5);
  });

  it('floors at zero rather than counting up past the deadline', () => {
    // The sweeper is about to cancel this order. "0m" is urgent; "-3m" is a
    // puzzle, and a dash would read as "no hurry".
    expect(minutesLeft(order({ accept_deadline: '2026-09-11T08:57:00Z' }), now)).toBe(0);
  });

  it('is absent when the order is not waiting on an answer', () => {
    expect(minutesLeft(order({ status: 'PREPARING' }), now)).toBeNull();
  });
});

describe('describeFailure', () => {
  it("passes the server's own words through on a conflict", () => {
    // These are the interesting failures: somebody else moved the order. The
    // server says which, in a sentence written for this reader, and replacing
    // it with "something went wrong" would lose the only useful part.
    const error = new ApiError(409, {
      code: 'conflict',
      message: 'this order is not waiting to be accepted',
      request_id: 'r1',
    });
    expect(describeFailure(error)).toBe('this order is not waiting to be accepted');
  });

  it('says the session ended rather than showing an authorization error', () => {
    const error = new ApiError(401, {
      code: 'unauthorized',
      message: 'token expired',
      request_id: 'r1',
    });
    expect(describeFailure(error)).toContain('sign in again');
  });

  it('does not leak an internal failure to a shop', () => {
    const error = new ApiError(500, {
      code: 'internal',
      message: 'pq: deadlock detected',
      request_id: 'r1',
    });
    expect(describeFailure(error)).toBe('Something went wrong. Please try again.');
  });
});

describe('statusLabel', () => {
  it('reads the lifecycle in the shop’s terms', () => {
    expect(statusLabel('PLACED')).toBe('Awaiting your answer');
    expect(statusLabel('READY_FOR_PICKUP')).toBe('Waiting for a driver');
  });

  it('shows an unknown status rather than hiding it', () => {
    // A server that has learned a state this build has not is a deployment
    // fact the shop should see.
    expect(statusLabel('QUANTUM')).toBe('QUANTUM');
  });
});
