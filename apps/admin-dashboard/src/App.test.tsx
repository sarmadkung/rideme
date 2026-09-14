import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';

describe('App shell', () => {
  it('shows the phone-OTP login screen when signed out', () => {
    // Nobody is signed in on mount, so the shell must show the login screen
    // rather than any operational screen a signed-out visitor should not see.
    render(<App />);
    expect(screen.getByRole('heading', { name: 'RideMe Admin' })).toBeDefined();
    expect(screen.getByPlaceholderText('03001234567')).toBeDefined();
    expect(screen.getByRole('button', { name: 'Send code' })).toBeDefined();
  });
});
