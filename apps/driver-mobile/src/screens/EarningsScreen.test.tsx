import { fireEvent, render, screen, waitFor } from '@testing-library/react-native';
import type { ApiClient } from '@platform/api-client';
import type { DriverBalance, DriverEarnings, DriverStanding } from '@platform/types';
import { EarningsScreen } from './EarningsScreen';

function anEarnings(overrides: Partial<DriverEarnings> = {}): DriverEarnings {
  const now = new Date().toISOString();
  return {
    today: { net: { amount_minor: 240000, currency: 'PKR' }, trips: 6, from: now, to: now },
    week: { net: { amount_minor: 1450000, currency: 'PKR' }, trips: 34, from: now, to: now },
    trips: [
      { job_id: 'j1', amount: { amount_minor: 40000, currency: 'PKR' }, at: now },
      { job_id: 'j2', amount: { amount_minor: 36000, currency: 'PKR' }, at: now },
    ],
    ...overrides,
  } as DriverEarnings;
}

function aBalance(overrides: Partial<DriverStanding> = {}): DriverBalance {
  return {
    standing: {
      owed: { amount_minor: 30000, currency: 'PKR' },
      cap: { amount_minor: 80000, currency: 'PKR' },
      warn: { amount_minor: 50000, currency: 'PKR' },
      clearing: { amount_minor: 0, currency: 'PKR' },
      blocked: false,
      warning: false,
      limited: true,
      ...overrides,
    },
    message: 'You are holding PKR 300.00 of your PKR 800.00 limit.',
  } as DriverBalance;
}

function aClient(
  driverEarnings: ApiClient['driverEarnings'],
  driverBalance: ApiClient['driverBalance'] = async () => aBalance(),
): ApiClient {
  return { driverEarnings, driverBalance } as unknown as ApiClient;
}

describe('EarningsScreen', () => {
  it('shows what the driver made today and this week', async () => {
    render(<EarningsScreen client={aClient(async () => anEarnings())} onBack={jest.fn()} />);

    expect(await screen.findByTestId('earnings-today')).toBeTruthy();
    expect(screen.getByTestId('earnings-week')).toBeTruthy();
    expect(screen.getByText('6 trips')).toBeTruthy();
    expect(screen.getByText('34 trips')).toBeTruthy();
  });

  it('says the figure is net of the platform fee', async () => {
    // BD-05 is a flat 20% commission, so this is not what the customer paid.
    // Saying so once is cheaper than answering it one driver at a time.
    render(<EarningsScreen client={aClient(async () => anEarnings())} onBack={jest.fn()} />);

    expect(await screen.findByTestId('earnings-note')).toHaveTextContent(/after the platform fee/i);
  });

  it('lists the trips behind the total', async () => {
    // A driver checking earnings is usually checking one trip they think was
    // underpaid, and a total alone cannot answer that.
    render(<EarningsScreen client={aClient(async () => anEarnings())} onBack={jest.fn()} />);

    expect(await screen.findByTestId('earnings-trip-0')).toBeTruthy();
    expect(screen.getByTestId('earnings-trip-1')).toBeTruthy();
  });

  it('never shows zero when the books could not be reached', async () => {
    // A driver shown PKR 0 because the ledger was unreachable would believe
    // they had worked a shift for nothing.
    render(
      <EarningsScreen
        client={aClient(async () => {
          throw new Error('unavailable');
        })}
        onBack={jest.fn()}
      />,
    );

    expect(await screen.findByTestId('earnings-error')).toBeTruthy();
    expect(screen.queryByTestId('earnings-today')).toBeNull();
    expect(screen.queryByText('PKR 0.00')).toBeNull();
  });

  it('can be retried after a failure', async () => {
    let calls = 0;
    const client = aClient(async () => {
      calls += 1;
      if (calls === 1) throw new Error('unavailable');
      return anEarnings();
    });
    render(<EarningsScreen client={client} onBack={jest.fn()} />);

    fireEvent.press(await screen.findByTestId('earnings-retry'));

    await waitFor(() => expect(screen.getByTestId('earnings-today')).toBeTruthy());
  });

  it('says so plainly when there were no trips', async () => {
    render(
      <EarningsScreen
        client={aClient(async () =>
          anEarnings({
            today: {
              net: { amount_minor: 0, currency: 'PKR' },
              trips: 0,
              from: new Date().toISOString(),
              to: new Date().toISOString(),
            },
            trips: [],
          } as Partial<DriverEarnings>),
        )}
        onBack={jest.fn()}
      />,
    );

    expect(await screen.findByTestId('earnings-empty')).toBeTruthy();
  });

  it('gets out of the way', async () => {
    const onBack = jest.fn();
    render(<EarningsScreen client={aClient(async () => anEarnings())} onBack={onBack} />);

    fireEvent.press(await screen.findByTestId('earnings-back'));
    expect(onBack).toHaveBeenCalled();
  });

  describe('platform cash the driver is holding', () => {
    // Shown above the earnings, and never subtracted from them. One is what
    // they made; the other is cash in their pocket that belongs to the
    // platform. A driver would reasonably read a netted figure as a deduction
    // from their pay.
    it('shows what is held, separately from what was earned', async () => {
      render(<EarningsScreen client={aClient(async () => anEarnings())} onBack={jest.fn()} />);

      expect(await screen.findByTestId('earnings-held-amount')).toHaveTextContent('PKR 300.00');
      // The earnings figure is untouched by it.
      expect(screen.getByTestId('earnings-today')).toBeTruthy();
    });

    // A driver with no cap has nothing to be told, and a card reading
    // "you are holding PKR 0.00" is noise on the screen they open to check
    // their pay.
    it('says nothing when no cap applies', async () => {
      const uncapped = aBalance();
      uncapped.standing.limited = false;
      render(
        <EarningsScreen
          client={aClient(
            async () => anEarnings(),
            async () => uncapped,
          )}
          onBack={jest.fn()}
        />,
      );

      await screen.findByTestId('earnings-today');
      expect(screen.queryByTestId('earnings-held')).toBeNull();
    });

    // The two questions are independent. Answering neither because one store
    // was unreachable is worse than answering one.
    it('still shows earnings when the balance cannot be read', async () => {
      render(
        <EarningsScreen
          client={aClient(
            async () => anEarnings(),
            async () => {
              throw new Error('books unreachable');
            },
          )}
          onBack={jest.fn()}
        />,
      );

      expect(await screen.findByTestId('earnings-today')).toBeTruthy();
      expect(screen.queryByTestId('earnings-held')).toBeNull();
    });
  });
});
