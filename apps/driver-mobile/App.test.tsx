import { render, screen } from '@testing-library/react-native';
import App from './App';

describe('Driver shell', () => {
  it('renders without crashing when the environment is absent', () => {
    render(<App />);
    expect(screen.getByTestId('app-root')).toBeTruthy();
    expect(screen.getByText('RideMe Driver')).toBeTruthy();
  });
});
