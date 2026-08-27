import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { App } from './App';

describe('App shell', () => {
  it('renders the placeholder and the validated environment', () => {
    render(<App />);
    expect(screen.getByRole('heading', { name: 'RideMe Admin' })).toBeDefined();
    expect(screen.getByTestId('app-env').textContent).toBe('test');
  });
});
